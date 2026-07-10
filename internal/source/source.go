package source

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type ResolvedAsset struct {
	Source    string
	Requested string
	Resolved  string
	AssetName string
	Kind      string
	URL       string
	Path      string
	SHA256    string
}

type Hasher interface {
	SHA256File(path string) (string, error)
	CopyFile(src, dst string) error
	CopyExact(src, dst string) error
}

type AssetRequest struct {
	Pattern string
	SHA256  string
}

type ResolveRequest struct {
	Source      string
	Assets      []AssetRequest
	Root        string
	DownloadDir string
	GitHubToken string
	Proxy       string
	Retries     int
	Progress    io.Writer
	Hasher      Hasher
	HTTPClient  *http.Client
}

// ResolveAsset 解析单个资源模式并返回唯一解析结果。
func ResolveAsset(source, pattern, root, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher) (ResolvedAsset, error) {
	resolved, err := ResolveAssetRequests(ResolveRequest{
		Source: source, Assets: []AssetRequest{{Pattern: pattern}}, Root: root, DownloadDir: downloadDir,
		GitHubToken: githubToken, Proxy: proxy, Retries: retries, Progress: progress, Hasher: h,
	})
	if err != nil {
		return ResolvedAsset{}, err
	}
	return resolved[0], nil
}

// ResolveAssets 根据来源类型解析多个资源模式并下载或复制到下载目录。
func ResolveAssets(source string, patterns []string, root, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher) ([]ResolvedAsset, error) {
	assets := make([]AssetRequest, 0, len(patterns))
	for _, pattern := range patterns {
		assets = append(assets, AssetRequest{Pattern: pattern})
	}
	return ResolveAssetRequests(ResolveRequest{
		Source: source, Assets: assets, Root: root, DownloadDir: downloadDir, GitHubToken: githubToken,
		Proxy: proxy, Retries: retries, Progress: progress, Hasher: h,
	})
}

// ResolveAssetRequests 根据来源类型解析多个资源请求并校验可选 checksum。
func ResolveAssetRequests(req ResolveRequest) ([]ResolvedAsset, error) {
	if req.Retries < 1 {
		return nil, fmt.Errorf("download retries must be at least 1")
	}
	if err := os.MkdirAll(req.DownloadDir, 0o755); err != nil {
		return nil, err
	}
	var resolved []ResolvedAsset
	var err error
	switch {
	case strings.HasPrefix(req.Source, "file:"):
		resolved, err = resolveEach(req.Assets, func(asset AssetRequest) (ResolvedAsset, error) {
			return resolveFile(req.Source, asset.Pattern, req.Root, req.DownloadDir, req.Hasher)
		})
	case strings.HasPrefix(req.Source, "url:"):
		client, clientErr := ensureHTTPClient(req.HTTPClient, req.Proxy)
		if clientErr != nil {
			return nil, clientErr
		}
		resolved, err = resolveEach(req.Assets, func(asset AssetRequest) (ResolvedAsset, error) {
			return resolveURL(req.Source, asset.Pattern, req.DownloadDir, req.GitHubToken, req.Proxy, req.Retries, req.Progress, req.Hasher, client)
		})
	case strings.HasPrefix(req.Source, "github:"):
		client, clientErr := ensureHTTPClient(req.HTTPClient, req.Proxy)
		if clientErr != nil {
			return nil, clientErr
		}
		resolved, err = resolveGitHubAssets(req.Source, req.Assets, req.DownloadDir, req.GitHubToken, req.Proxy, req.Retries, req.Progress, req.Hasher, client)
	default:
		return nil, fmt.Errorf("unsupported source: %s", req.Source)
	}
	if err != nil {
		return nil, err
	}
	for i, asset := range req.Assets {
		if err := verifyChecksum(asset.SHA256, resolved[i]); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

// resolveEach 逐个解析资源模式并拒绝多个模式命中同一资源。
func resolveEach(assets []AssetRequest, resolve func(AssetRequest) (ResolvedAsset, error)) ([]ResolvedAsset, error) {
	resolved := make([]ResolvedAsset, 0, len(assets))
	seen := map[string]ResolvedAsset{}
	for _, request := range assets {
		asset, err := resolve(request)
		if err != nil {
			return nil, err
		}
		if err := rejectDuplicateResolvedAsset(seen, asset); err != nil {
			return nil, err
		}
		resolved = append(resolved, asset)
	}
	return resolved, nil
}

// rejectDuplicateResolvedAsset 记录已解析资源并拒绝重复命中的资产。
func rejectDuplicateResolvedAsset(seen map[string]ResolvedAsset, asset ResolvedAsset) error {
	key := resolvedAssetKey(asset)
	if previous, ok := seen[key]; ok {
		return fmt.Errorf("multiple asset patterns resolved to the same file: %s@%s/%s", previous.Source, previous.Resolved, previous.AssetName)
	}
	seen[key] = asset
	return nil
}

// resolvedAssetKey 生成用于判重的资源身份键。
func resolvedAssetKey(asset ResolvedAsset) string {
	return asset.Source + "\x00" + asset.Resolved + "\x00" + asset.AssetName
}

func verifyChecksum(expected string, asset ResolvedAsset) error {
	if expected == "" {
		return nil
	}
	if asset.Kind == "dir" {
		return fmt.Errorf("sha256 is not supported for directory asset: %s", asset.AssetName)
	}
	if !strings.EqualFold(expected, asset.SHA256) {
		return fmt.Errorf("sha256 mismatch for %s: got %s, want %s", asset.AssetName, asset.SHA256, strings.ToLower(expected))
	}
	return nil
}
