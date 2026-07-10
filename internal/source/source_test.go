package source

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepack/internal/fsutil"
)

type testHasher struct{}

// SHA256File 实现测试哈希器的对应接口。
func (testHasher) SHA256File(path string) (string, error) {
	return "sha", nil
}

// CopyFile 实现测试哈希器的对应接口。
func (testHasher) CopyFile(src, dst string) error {
	return fsutil.CopyFile(src, dst)
}

// CopyExact 实现测试哈希器的对应接口。
func (testHasher) CopyExact(src, dst string) error {
	return fsutil.CopyExact(src, dst)
}

// TestResolveFileSource 验证本地目录 source 可以按 asset 模式解析文件。
func TestResolveFileSource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixtures", "asset.bin"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveAsset("file:fixtures", "asset\\.bin", root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.AssetName != "asset.bin" || resolved.Requested != "local" || resolved.Resolved != "local" {
		t.Fatalf("unexpected resolved asset: %+v", resolved)
	}
}

// TestResolveFileSourceDirectoryWithoutPattern 验证目录型 file: source 可以直接作为资产。
func TestResolveFileSourceDirectoryWithoutPattern(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "bin", "run.txt"), []byte("run"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveAsset("file:app", "", root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.AssetName != "app" || resolved.Kind != "dir" || resolved.SHA256 != "" {
		t.Fatalf("unexpected directory asset: %+v", resolved)
	}
	if _, err := os.Stat(filepath.Join(root, "downloads", "app", "bin", "run.txt")); err != nil {
		t.Fatalf("expected copied directory asset: %v", err)
	}
}

// TestResolveFileSourceMatchesDirectoryAsset 验证目录型 file: source 可匹配直接子目录资产。
func TestResolveFileSourceMatchesDirectoryAsset(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fixtures", "app", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixtures", "app", "bin", "run.txt"), []byte("run"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveAsset("file:fixtures", `^app$`, root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.AssetName != "app" || resolved.Kind != "dir" {
		t.Fatalf("unexpected matched directory asset: %+v", resolved)
	}
}

// TestResolveFileSourceRejectsMultipleMatches 验证对应场景的行为。
func TestResolveFileSourceRejectsMultipleMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"payload-a.bin", "payload-b.bin"} {
		if err := os.WriteFile(filepath.Join(root, "fixtures", name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ResolveAsset("file:fixtures", `^payload-.*\.bin$`, root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{}); err == nil {
		t.Fatal("expected multiple local asset matches to fail")
	}
}

// TestResolveAssetsRejectsDuplicateLocalFile 验证对应场景的行为。
func TestResolveAssetsRejectsDuplicateLocalFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixtures", "asset.bin"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveAssets("file:fixtures", []string{`asset\.bin`, `^asset\.bin$`}, root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{})
	if err == nil || !strings.Contains(err.Error(), "multiple asset patterns resolved to the same file") {
		t.Fatalf("expected duplicate resolved asset error, got %v", err)
	}
}

// TestResolveAssetsRejectsDuplicateURLFile 验证对应场景的行为。
func TestResolveAssetsRejectsDuplicateURLFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("asset"))
	}))
	defer server.Close()
	_, err := ResolveAssets("url:"+server.URL+"/payload.bin", []string{`payload\.bin`, `^payload\.bin$`}, t.TempDir(), filepath.Join(t.TempDir(), "downloads"), "", "", 3, io.Discard, testHasher{})
	if err == nil || !strings.Contains(err.Error(), "multiple asset patterns resolved to the same file") {
		t.Fatalf("expected duplicate resolved asset error, got %v", err)
	}
}

// TestResolveFileSourceRejectsSymlink 验证对应场景的行为。
func TestResolveFileSourceRejectsSymlink(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("symlink creation on Windows may require privileges")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixtures", "target.bin"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "fixtures", "target.bin"), filepath.Join(root, "fixtures", "asset.bin")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveAsset("file:fixtures/asset.bin", "asset\\.bin", root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{}); err == nil {
		t.Fatal("expected symlink file source to fail")
	}
}

// TestResolveFileMissingSourceError 验证本地 source 不存在时报告 does not exist。
func TestResolveFileMissingSourceError(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveAsset("file:missing.bin", ".*", root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{})
	if err == nil || !strings.Contains(err.Error(), "local source does not exist") {
		t.Fatalf("expected missing source error, got %v", err)
	}
}

// TestResolveFileInvalidTypeError 验证本地 source 是非法类型时不会被伪装成不存在。
func TestResolveFileInvalidTypeError(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("symlink creation on Windows may require privileges")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target.bin"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "target.bin"), filepath.Join(root, "link.bin")); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveAsset("file:link.bin", ".*", root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{})
	if err == nil || !strings.Contains(err.Error(), "invalid local source") || strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected invalid source error, got %v", err)
	}
}

// TestResolveFileDirMatchesBeforeValidate 验证目录型 file: source 先匹配文件名；不匹配的 symlink 不影响，匹配到 symlink 仍失败。
func TestResolveFileDirMatchesBeforeValidate(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("symlink creation on Windows may require privileges")
	}
	root := t.TempDir()
	fixtures := filepath.Join(root, "fixtures")
	if err := os.MkdirAll(fixtures, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtures, "wanted.zip"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtures, "target.bin"), []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(fixtures, "target.bin"), filepath.Join(fixtures, "unrelated-link")); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveAsset("file:fixtures", `^wanted\.zip$`, root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.AssetName != "wanted.zip" {
		t.Fatalf("asset = %s, want wanted.zip", resolved.AssetName)
	}

	_, err = ResolveAsset("file:fixtures", `^unrelated-link$`, root, filepath.Join(root, "downloads2"), "", "", 3, io.Discard, testHasher{})
	if err == nil {
		t.Fatal("expected matched symlink to fail")
	}
}

// TestResolveURLSource 验证直接 URL source 可以下载并记录解析结果。
func TestResolveURLSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("asset"))
	}))
	defer server.Close()
	root := t.TempDir()
	var progress bytes.Buffer
	resolved, err := ResolveAsset("url:"+server.URL+"/payload.bin", "payload\\.bin", root, filepath.Join(root, "downloads"), "", "", 3, &progress, testHasher{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.AssetName != "payload.bin" || resolved.Requested != "direct" {
		t.Fatalf("unexpected resolved asset: %+v", resolved)
	}
	if progress.Len() == 0 {
		t.Fatal("expected download progress to use injected writer")
	}
}

// TestResolveURLRejectsUnsupportedSchemes 验证对应场景的行为。
func TestResolveURLRejectsUnsupportedSchemes(t *testing.T) {
	for _, source := range []string{
		"url:ftp://example.com/file.zip",
		"url:file:///tmp/file.zip",
		"url:not-a-url",
	} {
		if _, err := ResolveAsset(source, ".*", t.TempDir(), filepath.Join(t.TempDir(), "downloads"), "", "", 1, io.Discard, testHasher{}); err == nil {
			t.Fatalf("expected %s to fail", source)
		}
	}
}

// TestResolveInvalidGitHubSource 验证格式错误的 GitHub source 会被拒绝。
func TestResolveInvalidGitHubSource(t *testing.T) {
	if _, err := ResolveAsset("github:bad", ".*", t.TempDir(), t.TempDir(), "", "", 3, io.Discard, testHasher{}); err == nil {
		t.Fatal("expected invalid github source error")
	}
}

// TestResolveFileRejectsTraversal 验证 file source 不能穿越到 kit root 外。
func TestResolveFileRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveAsset("file:../outside.bin", ".*", root, filepath.Join(root, "downloads"), "", "", 3, io.Discard, testHasher{}); err == nil {
		t.Fatal("expected file traversal to fail")
	}
}

// TestExplicitProxyConfiguresTransport 验证显式代理会写入 HTTP transport。
func TestExplicitProxyConfiguresTransport(t *testing.T) {
	client, err := newHTTPClient("socks5://127.0.0.1:7890")
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/file.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL.String() != "socks5://127.0.0.1:7890" {
		t.Fatalf("unexpected proxy URL: %s", proxyURL)
	}
}

// TestDefaultHTTPClientUsesDefaultTransport 验证未指定代理时保留默认环境代理行为。
func TestDefaultHTTPClientUsesDefaultTransport(t *testing.T) {
	client, err := newHTTPClient("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.Transport.(*http.Transport); !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
}

// TestDownloadRetriesRetryableStatus 验证对应场景的行为。
func TestDownloadRetriesRetryableStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("asset"))
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "asset.bin")
	if err := download(server.URL+"/asset.bin", target, nil, "", 3, io.Discard); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

// TestDownloadRetriesOneDoesNotRetry 验证对应场景的行为。
func TestDownloadRetriesOneDoesNotRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "asset.bin")
	if err := download(server.URL+"/asset.bin", target, nil, "", 1, io.Discard); err == nil {
		t.Fatal("expected download to fail")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

// TestDownloadRetriesTwoSucceedsAfterOneRetry 验证对应场景的行为。
func TestDownloadRetriesTwoSucceedsAfterOneRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("asset"))
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "asset.bin")
	if err := download(server.URL+"/asset.bin", target, nil, "", 2, io.Discard); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

// TestResolveGitHubAPIErrors 验证对应场景的行为。
func TestResolveGitHubAPIErrors(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		status      int
		headers     map[string]string
		wantParts   []string
		wantEscaped string
	}{
		{
			name:      "latest 404",
			source:    "github:owner/repo",
			status:    http.StatusNotFound,
			wantParts: []string{"latest release not found", "request=latest release", "status 404"},
		},
		{
			name:        "tag 404",
			source:      "github:owner/repo@v/a b#c",
			status:      http.StatusNotFound,
			wantParts:   []string{"release tag not found: v/a b#c", "request=release tag", "status 404"},
			wantEscaped: "/repos/owner/repo/releases/tags/v%2Fa%20b%23c",
		},
		{
			name:   "rate limit",
			source: "github:owner/repo",
			status: http.StatusForbidden,
			headers: map[string]string{
				"X-RateLimit-Remaining": "0",
				"X-RateLimit-Reset":     "1893456000",
				"Retry-After":           "60",
			},
			wantParts: []string{"GitHub API failed", "request=latest release", "rate_limit_remaining=0", "rate_limit_reset=2030-01-01T00:00:00Z", "retry_after=60"},
		},
		{
			name:      "429 retry after",
			source:    "github:owner/repo",
			status:    http.StatusTooManyRequests,
			headers:   map[string]string{"Retry-After": "0"},
			wantParts: []string{"GitHub API failed", "request=latest release", "retry_after=0"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.wantEscaped != "" && r.URL.EscapedPath() != tc.wantEscaped {
					t.Fatalf("escaped path = %q, want %q", r.URL.EscapedPath(), tc.wantEscaped)
				}
				for key, value := range tc.headers {
					w.Header().Set(key, value)
				}
				http.Error(w, "api error", tc.status)
			}))
			defer server.Close()
			oldBase := githubAPIBase
			githubAPIBase = server.URL
			defer func() { githubAPIBase = oldBase }()

			_, err := ResolveAsset(tc.source, "asset\\.zip", t.TempDir(), filepath.Join(t.TempDir(), "downloads"), "secret-token", "", 1, io.Discard, testHasher{})
			if err == nil {
				t.Fatal("expected GitHub API error")
			}
			for _, want := range tc.wantParts {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error missing %q:\n%s", want, err.Error())
				}
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("error leaked token: %s", err.Error())
			}
		})
	}
}

// TestResolveGitHubUsesRetryCountForAPIAndAsset 验证对应场景的行为。
func TestResolveGitHubUsesRetryCountForAPIAndAsset(t *testing.T) {
	apiAttempts := 0
	assetAttempts := 0
	serverURL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"):
			apiAttempts++
			if apiAttempts == 1 {
				w.Header().Set("Retry-After", "0")
				http.Error(w, "busy", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprintf(w, `{"tag_name":"v1","assets":[{"name":"asset.zip","browser_download_url":%q}]}`, serverURL+"/asset.zip")
		case r.URL.Path == "/asset.zip":
			assetAttempts++
			if assetAttempts == 1 {
				w.Header().Set("Retry-After", "0")
				http.Error(w, "busy", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("asset"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	oldBase := githubAPIBase
	githubAPIBase = server.URL
	defer func() { githubAPIBase = oldBase }()
	if _, err := ResolveAsset("github:owner/repo", "asset\\.zip", t.TempDir(), filepath.Join(t.TempDir(), "downloads"), "", "", 2, io.Discard, testHasher{}); err != nil {
		t.Fatal(err)
	}
	if apiAttempts != 2 || assetAttempts != 2 {
		t.Fatalf("attempts api=%d asset=%d, want 2/2", apiAttempts, assetAttempts)
	}
}

// TestResolveGitHubAssetsResolveReleaseOnce 验证对应场景的行为。
func TestResolveGitHubAssetsResolveReleaseOnce(t *testing.T) {
	apiAttempts := 0
	assetAttempts := 0
	serverURL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"):
			apiAttempts++
			fmt.Fprintf(w, `{"tag_name":"v1","assets":[{"name":"tool-a.bin","browser_download_url":%q},{"name":"tool-b.bin","browser_download_url":%q}]}`, serverURL+"/tool-a.bin", serverURL+"/tool-b.bin")
		case r.URL.Path == "/tool-a.bin" || r.URL.Path == "/tool-b.bin":
			assetAttempts++
			_, _ = w.Write([]byte("asset"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	oldBase := githubAPIBase
	githubAPIBase = server.URL
	defer func() { githubAPIBase = oldBase }()
	resolved, err := ResolveAssets("github:owner/repo", []string{"tool-a\\.bin", "tool-b\\.bin"}, t.TempDir(), filepath.Join(t.TempDir(), "downloads"), "", "", 1, io.Discard, testHasher{})
	if err != nil {
		t.Fatal(err)
	}
	if apiAttempts != 1 || assetAttempts != 2 {
		t.Fatalf("attempts api=%d asset=%d, want 1/2", apiAttempts, assetAttempts)
	}
	if len(resolved) != 2 || resolved[0].Resolved != "v1" || resolved[1].Resolved != "v1" {
		t.Fatalf("unexpected resolved assets: %+v", resolved)
	}
}

// TestResolveGitHubAssetsDoNotMixLatestReleaseChanges 验证对应场景的行为。
func TestResolveGitHubAssetsDoNotMixLatestReleaseChanges(t *testing.T) {
	apiAttempts := 0
	serverURL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"):
			apiAttempts++
			if apiAttempts == 1 {
				fmt.Fprintf(w, `{"tag_name":"v1","assets":[{"name":"tool-a-v1.bin","browser_download_url":%q},{"name":"tool-b-v1.bin","browser_download_url":%q}]}`, serverURL+"/tool-a-v1.bin", serverURL+"/tool-b-v1.bin")
				return
			}
			fmt.Fprintf(w, `{"tag_name":"v2","assets":[{"name":"tool-a-v2.bin","browser_download_url":%q},{"name":"tool-b-v2.bin","browser_download_url":%q}]}`, serverURL+"/tool-a-v2.bin", serverURL+"/tool-b-v2.bin")
		case strings.HasPrefix(r.URL.Path, "/tool-"):
			_, _ = w.Write([]byte("asset"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	oldBase := githubAPIBase
	githubAPIBase = server.URL
	defer func() { githubAPIBase = oldBase }()
	resolved, err := ResolveAssets("github:owner/repo", []string{"tool-a.*\\.bin", "tool-b.*\\.bin"}, t.TempDir(), filepath.Join(t.TempDir(), "downloads"), "", "", 1, io.Discard, testHasher{})
	if err != nil {
		t.Fatal(err)
	}
	if apiAttempts != 1 {
		t.Fatalf("release API attempts = %d, want 1", apiAttempts)
	}
	for _, asset := range resolved {
		if asset.Resolved != "v1" || strings.Contains(asset.AssetName, "v2") {
			t.Fatalf("mixed release asset: %+v", asset)
		}
	}
}

// TestResolveGitHubAssetsRejectsDuplicateAsset 验证对应场景的行为。
func TestResolveGitHubAssetsRejectsDuplicateAsset(t *testing.T) {
	apiAttempts := 0
	assetAttempts := 0
	serverURL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"):
			apiAttempts++
			fmt.Fprintf(w, `{"tag_name":"v1","assets":[{"name":"tool.bin","browser_download_url":%q}]}`, serverURL+"/tool.bin")
		case r.URL.Path == "/tool.bin":
			assetAttempts++
			_, _ = w.Write([]byte("asset"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	oldBase := githubAPIBase
	githubAPIBase = server.URL
	defer func() { githubAPIBase = oldBase }()
	_, err := ResolveAssets("github:owner/repo", []string{"tool\\.bin", "^tool\\.bin$"}, t.TempDir(), filepath.Join(t.TempDir(), "downloads"), "", "", 1, io.Discard, testHasher{})
	if err == nil || !strings.Contains(err.Error(), "multiple asset patterns resolved to the same file") {
		t.Fatalf("expected duplicate resolved asset error, got %v", err)
	}
	if apiAttempts != 1 || assetAttempts != 1 {
		t.Fatalf("attempts api=%d asset=%d, want 1/1", apiAttempts, assetAttempts)
	}
}
