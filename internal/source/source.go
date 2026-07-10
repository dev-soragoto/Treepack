package source

import (
	"fmt"
	"io"
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

// ResolveAsset 解析单个资源模式并返回唯一解析结果。
func ResolveAsset(source, pattern, root, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher) (ResolvedAsset, error) {
	resolved, err := ResolveAssets(source, []string{pattern}, root, downloadDir, githubToken, proxy, retries, progress, h)
	if err != nil {
		return ResolvedAsset{}, err
	}
	return resolved[0], nil
}

// ResolveAssets 根据来源类型解析多个资源模式并下载或复制到下载目录。
func ResolveAssets(source string, patterns []string, root, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher) ([]ResolvedAsset, error) {
	if retries < 1 {
		return nil, fmt.Errorf("download retries must be at least 1")
	}
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return nil, err
	}
	switch {
	case strings.HasPrefix(source, "file:"):
		return resolveEach(patterns, func(pattern string) (ResolvedAsset, error) {
			return resolveFile(source, pattern, root, downloadDir, h)
		})
	case strings.HasPrefix(source, "url:"):
		return resolveEach(patterns, func(pattern string) (ResolvedAsset, error) {
			return resolveURL(source, pattern, downloadDir, githubToken, proxy, retries, progress, h)
		})
	case strings.HasPrefix(source, "github:"):
		return resolveGitHubAssets(source, patterns, downloadDir, githubToken, proxy, retries, progress, h)
	default:
		return nil, fmt.Errorf("unsupported source: %s", source)
	}
}

// resolveEach 逐个解析资源模式并拒绝多个模式命中同一资源。
func resolveEach(patterns []string, resolve func(string) (ResolvedAsset, error)) ([]ResolvedAsset, error) {
	resolved := make([]ResolvedAsset, 0, len(patterns))
	seen := map[string]ResolvedAsset{}
	for _, pattern := range patterns {
		asset, err := resolve(pattern)
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
