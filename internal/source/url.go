package source

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"treepack/internal/fsutil"
)

// resolveURL 从 url: 来源下载资源并校验可选的文件名匹配模式。
func resolveURL(source string, request AssetRequest, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher, client *http.Client, cache cacheConfig) (ResolvedAsset, error) {
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
	if request.Pattern != "" {
		ok, err := regexp.MatchString(request.Pattern, name)
		if err != nil {
			return ResolvedAsset{}, err
		}
		if !ok {
			return ResolvedAsset{}, fmt.Errorf("url filename %q does not match asset pattern %q", name, request.Pattern)
		}
	}
	target, err := fsutil.ResolveUnder(downloadDir, name)
	if err != nil {
		return ResolvedAsset{}, err
	}
	key := cacheKey("url", rawURL, strings.ToLower(request.SHA256))
	if request.SHA256 != "" {
		if sum, ok := cacheRead(cache, key, target, name, -1, "", request.SHA256, h); ok {
			return ResolvedAsset{Source: source, Requested: "direct", Resolved: "direct", AssetName: name, Kind: "file", URL: rawURL, Path: target, SHA256: sum}, nil
		}
	}
	if err := download(rawURL, target, headersForURL(rawURL, githubToken), proxy, retries, progress, client); err != nil {
		return ResolvedAsset{}, err
	}
	sum, err := h.SHA256File(target)
	if err != nil {
		return ResolvedAsset{}, err
	}
	if request.SHA256 != "" && strings.EqualFold(request.SHA256, sum) {
		if info, statErr := os.Stat(target); statErr == nil {
			cacheWrite(cache, key, target, cacheMeta(name, info.Size(), sum, ""), h)
		}
	}
	return ResolvedAsset{Source: source, Requested: "direct", Resolved: "direct", AssetName: name, Kind: "file", URL: rawURL, Path: target, SHA256: sum}, nil
}
