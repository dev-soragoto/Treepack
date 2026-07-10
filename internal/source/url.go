package source

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"treepack/internal/fsutil"
)

// resolveURL 从 url: 来源下载资源并校验可选的文件名匹配模式。
func resolveURL(source, pattern, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher) (ResolvedAsset, error) {
	rawURL := strings.TrimPrefix(source, "url:")
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ResolvedAsset{}, err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return ResolvedAsset{}, fmt.Errorf("url source only supports http and https")
	}
	if parsed.Host == "" {
		return ResolvedAsset{}, fmt.Errorf("url source requires host")
	}
	name := filepath.Base(parsed.Path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "download"
	}
	if pattern != "" {
		ok, err := regexp.MatchString(pattern, name)
		if err != nil {
			return ResolvedAsset{}, err
		}
		if !ok {
			return ResolvedAsset{}, fmt.Errorf("url filename %q does not match asset pattern %q", name, pattern)
		}
	}
	target, err := fsutil.ResolveUnder(downloadDir, name)
	if err != nil {
		return ResolvedAsset{}, err
	}
	if err := download(rawURL, target, headersForURL(rawURL, githubToken), proxy, retries, progress); err != nil {
		return ResolvedAsset{}, err
	}
	sum, err := h.SHA256File(target)
	if err != nil {
		return ResolvedAsset{}, err
	}
	return ResolvedAsset{Source: source, Requested: "direct", Resolved: "direct", AssetName: name, Kind: "file", URL: rawURL, Path: target, SHA256: sum}, nil
}
