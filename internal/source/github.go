package source

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"treepack/internal/fsutil"
)

var githubAPIBase = "https://api.github.com"

type githubRelease struct {
	ID      int64  `json:"id"`
	TagName string `json:"tag_name"`
}

type githubAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// resolveGitHubAssets 解析 GitHub release 资源并下载所有匹配的唯一资产。
func resolveGitHubAssets(source string, requests []AssetRequest, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher, client *http.Client, cache cacheConfig) ([]ResolvedAsset, error) {
	spec := strings.TrimPrefix(source, "github:")
	repoPart, tag, hasTag := strings.Cut(spec, "@")
	parts := strings.Split(repoPart, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid github source: %s", source)
	}
	if !validGitHubName(parts[0]) || !validGitHubName(parts[1]) {
		return nil, fmt.Errorf("invalid github source: %s", source)
	}
	for _, request := range requests {
		if request.Pattern == "" {
			return nil, fmt.Errorf("github source needs an asset pattern: %s", source)
		}
	}
	api := fmt.Sprintf("%s/repos/%s/%s/releases/latest", strings.TrimRight(githubAPIBase, "/"), parts[0], parts[1])
	requestKind := "latest release"
	if hasTag {
		api = fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", strings.TrimRight(githubAPIBase, "/"), parts[0], parts[1], url.PathEscape(tag))
		requestKind = "release tag"
	}
	var rel githubRelease
	if err := getJSON(api, &rel, githubHeaders(githubToken, true), proxy, retries, requestKind, tag, client); err != nil {
		return nil, fmt.Errorf("cannot resolve GitHub release %s/%s: %w", parts[0], parts[1], err)
	}
	releaseTag := rel.TagName
	if releaseTag == "" {
		if hasTag {
			releaseTag = tag
		} else {
			releaseTag = "unknown"
		}
	}
	assets, err := listGitHubReleaseAssets(parts[0], parts[1], rel.ID, githubToken, proxy, retries, client)
	if err != nil {
		return nil, fmt.Errorf("cannot list GitHub release assets %s/%s@%s: %w", parts[0], parts[1], releaseTag, err)
	}
	resolved := make([]ResolvedAsset, 0, len(requests))
	seen := map[string]ResolvedAsset{}
	for _, request := range requests {
		re, err := regexp.Compile(request.Pattern)
		if err != nil {
			return nil, err
		}
		var matches []githubAsset
		for _, asset := range assets {
			if re.MatchString(asset.Name) && asset.BrowserDownloadURL != "" {
				matches = append(matches, asset)
			}
		}
		if len(matches) > 1 {
			names := make([]string, 0, len(matches))
			for _, asset := range matches {
				names = append(names, asset.Name)
			}
			return nil, fmt.Errorf("multiple GitHub assets matched %q in %s/%s@%s: %s", request.Pattern, parts[0], parts[1], releaseTag, strings.Join(names, ", "))
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no GitHub asset matched %q in %s/%s@%s", request.Pattern, parts[0], parts[1], releaseTag)
		}
		asset := matches[0]
		requested := "latest"
		if hasTag {
			requested = tag
		}
		resolvedAsset := ResolvedAsset{Source: source, Requested: requested, Resolved: releaseTag, AssetName: asset.Name, Kind: "file", URL: asset.BrowserDownloadURL}
		if err := rejectDuplicateResolvedAsset(seen, resolvedAsset); err != nil {
			return nil, err
		}
		target, err := fsutil.ResolveUnder(downloadDir, asset.Name)
		if err != nil {
			return nil, err
		}
		key := cacheKey("github", parts[0]+"/"+parts[1], releaseTag, fmt.Sprint(asset.ID))
		expected := request.SHA256
		if expected == "" {
			expected = normalizedDigestSHA(asset.Digest)
		}
		assetSize := asset.Size
		if assetSize == 0 {
			assetSize = -1
		}
		if sum, ok := cacheRead(cache, key, target, asset.Name, assetSize, asset.Digest, expected, h); ok {
			resolvedAsset.Path, resolvedAsset.SHA256 = target, sum
			resolved = append(resolved, resolvedAsset)
			continue
		}
		if err := download(asset.BrowserDownloadURL, target, githubHeaders(githubToken, false), proxy, retries, progress, client); err != nil {
			return nil, err
		}
		sum, err := h.SHA256File(target)
		if err != nil {
			return nil, err
		}
		resolvedAsset.Path = target
		resolvedAsset.SHA256 = sum
		if digestSHA := normalizedDigestSHA(asset.Digest); digestSHA != "" && !strings.EqualFold(digestSHA, sum) {
			return nil, fmt.Errorf("GitHub digest mismatch for %s: got %s, want %s", asset.Name, sum, digestSHA)
		}
		if info, statErr := os.Stat(target); statErr == nil && asset.Size > 0 && info.Size() != asset.Size {
			return nil, fmt.Errorf("GitHub asset size mismatch for %s: got %d, want %d", asset.Name, info.Size(), asset.Size)
		}
		if info, statErr := os.Stat(target); statErr == nil && (request.SHA256 == "" || strings.EqualFold(request.SHA256, sum)) {
			cacheWrite(cache, key, target, cacheMeta(asset.Name, info.Size(), sum, asset.Digest), h)
		}
		resolved = append(resolved, resolvedAsset)
	}
	return resolved, nil
}

func listGitHubReleaseAssets(owner, repo string, releaseID int64, githubToken, proxy string, retries int, client *http.Client) ([]githubAsset, error) {
	if releaseID == 0 {
		return nil, fmt.Errorf("release id is missing")
	}
	var all []githubAsset
	for page := 1; ; page++ {
		api := fmt.Sprintf("%s/repos/%s/%s/releases/%d/assets?per_page=100&page=%d", strings.TrimRight(githubAPIBase, "/"), owner, repo, releaseID, page)
		resp, attempts, err := doWithRetry(client, http.MethodGet, api, githubHeaders(githubToken, true), retries)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			err := httpError("release assets", "GitHub release assets failed", api, resp, attempts)
			resp.Body.Close()
			return nil, err
		}
		var pageAssets []githubAsset
		err = json.NewDecoder(resp.Body).Decode(&pageAssets)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, pageAssets...)
		if len(pageAssets) < 100 || !hasNextPage(resp.Header.Get("Link")) {
			break
		}
	}
	return all, nil
}

func hasNextPage(link string) bool {
	for _, part := range strings.Split(link, ",") {
		if strings.Contains(part, `rel="next"`) {
			return true
		}
	}
	return false
}

// validGitHubName 校验 GitHub owner 和 repo 名称只包含允许字符。
func validGitHubName(value string) bool {
	if value == "." || value == ".." || len(value) > 100 {
		return false
	}
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return false
	}
	return true
}
