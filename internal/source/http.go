package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxRetryAfter = 120 * time.Second

// download 通过 HTTP 下载资源到目标路径，并使用临时文件保证失败清理。
func download(rawURL, target string, headers map[string]string, proxy string, retries int, progressWriter io.Writer, clients ...*http.Client) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	part := target + ".part"
	_ = os.Remove(part)
	_ = os.Remove(target)
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(part)
			_ = os.Remove(target)
		}
	}()
	var injected *http.Client
	if len(clients) > 0 {
		injected = clients[0]
	}
	client, err := ensureHTTPClient(injected, proxy)
	if err != nil {
		return err
	}
	resp, attempts, err := doWithRetry(client, http.MethodGet, rawURL, headers, retries)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpError("download", "download failed", rawURL, resp, attempts)
	}
	out, err := os.Create(part)
	if err != nil {
		return err
	}
	name := filepath.Base(target)
	progress := newDownloadProgress(name, resp.ContentLength, progressWriter)
	_, copyErr := io.Copy(out, io.TeeReader(resp.Body, progress))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(part, target); err != nil {
		return err
	}
	cleanup = false
	progress.done()
	return nil
}

// getJSON 执行带重试的 JSON GET 请求并解码响应体。
func getJSON(rawURL string, target any, headers map[string]string, proxy string, retries int, requestKind, tag string, client *http.Client) error {
	client, err := ensureHTTPClient(client, proxy)
	if err != nil {
		return err
	}
	resp, attempts, err := doWithRetry(client, http.MethodGet, rawURL, headers, retries)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := "GitHub API failed"
		if resp.StatusCode == http.StatusNotFound {
			if requestKind == "latest release" {
				message = "latest release not found"
			} else {
				message = "release tag not found: " + tag
			}
		}
		return httpError(requestKind, message, rawURL, resp, attempts)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

// doWithRetry 对可重试的网络错误或状态码执行 HTTP 请求重试。
func doWithRetry(client *http.Client, method, rawURL string, headers map[string]string, maxAttempts int) (*http.Response, int, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(context.Background(), method, rawURL, nil)
		if err != nil {
			return nil, attempt, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == maxAttempts {
				return nil, attempt, fmt.Errorf("%s after %d attempt(s): %w", hostForError(rawURL), attempt, err)
			}
			delay, err := retryDelay(nil)
			if err != nil {
				return nil, attempt, err
			}
			time.Sleep(delay)
			continue
		}
		if !retryableStatus(resp.StatusCode) || attempt == maxAttempts {
			return resp, attempt, nil
		}
		delay, err := retryDelay(resp)
		resp.Body.Close()
		if err != nil {
			return nil, attempt, err
		}
		time.Sleep(delay)
	}
	return nil, maxAttempts, lastErr
}

// retryableStatus 判断 HTTP 状态码是否适合重试。
func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// retryDelay 依据 Retry-After 响应头或默认值计算重试等待时间。
func retryDelay(resp *http.Response) (time.Duration, error) {
	if resp != nil {
		if value := resp.Header.Get("Retry-After"); value != "" {
			if seconds, err := time.ParseDuration(value + "s"); err == nil {
				return limitRetryAfter(seconds)
			}
			if when, err := http.ParseTime(value); err == nil {
				if delay := time.Until(when); delay > 0 {
					return limitRetryAfter(delay)
				}
			}
		}
	}
	return 100 * time.Millisecond, nil
}

func limitRetryAfter(delay time.Duration) (time.Duration, error) {
	if delay > maxRetryAfter {
		return 0, fmt.Errorf("Retry-After exceeds maximum %s", maxRetryAfter)
	}
	return delay, nil
}

// httpError 构造包含主机、状态码、重试次数和限流信息的 HTTP 错误。
func httpError(kind, message, rawURL string, resp *http.Response, attempts int) error {
	details := []string{
		fmt.Sprintf("%s for %s: status %d after %d attempt(s)", message, hostForError(rawURL), resp.StatusCode, attempts),
		"request=" + kind,
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
			details = append(details, "rate_limit_remaining="+remaining)
		}
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if seconds, err := strconv.ParseInt(reset, 10, 64); err == nil {
				details = append(details, "rate_limit_reset="+time.Unix(seconds, 0).UTC().Format(time.RFC3339))
			} else {
				details = append(details, "rate_limit_reset="+reset)
			}
		}
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			details = append(details, "retry_after="+retryAfter)
		}
	}
	return fmt.Errorf("%s", strings.Join(details, "; "))
}

// hostForError 从 URL 中提取用于错误消息展示的主机名。
func hostForError(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL
	}
	return parsed.Host
}

// NewHTTPClient 创建带超时和可选代理配置的 HTTP 客户端。
func NewHTTPClient(proxy string) (*http.Client, error) {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport is not an *http.Transport")
	}
	clone := transport.Clone()
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	clone.DialContext = dialer.DialContext
	clone.TLSHandshakeTimeout = 15 * time.Second
	clone.ResponseHeaderTimeout = 30 * time.Second
	clone.ExpectContinueTimeout = 1 * time.Second
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		clone.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Transport: clone}, nil
}

func ensureHTTPClient(client *http.Client, proxy string) (*http.Client, error) {
	if client != nil {
		return client, nil
	}
	return NewHTTPClient(proxy)
}

func newHTTPClient(proxy string) (*http.Client, error) {
	return NewHTTPClient(proxy)
}

// githubHeaders 构造 GitHub API 或资产下载请求头。
func githubHeaders(token string, acceptJSON bool) map[string]string {
	headers := map[string]string{"User-Agent": "treepack/1.0"}
	if acceptJSON {
		headers["Accept"] = "application/vnd.github+json"
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}

// headersForURL 为普通 URL 下载构造请求头，并仅对 GitHub 资产附加令牌。
func headersForURL(rawURL, token string) map[string]string {
	headers := map[string]string{"User-Agent": "treepack/1.0"}
	if token == "" {
		return headers
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return headers
	}
	switch parsed.Hostname() {
	case "github.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}
