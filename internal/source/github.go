package source

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"treepack/internal/fsutil"
)

var githubAPIBase = "https://api.github.com"

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// resolveGitHub 解析单个 GitHub release 资源模式。
func resolveGitHub(source, pattern, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher) (ResolvedAsset, error) {
	resolved, err := resolveGitHubAssets(source, []string{pattern}, downloadDir, githubToken, proxy, retries, progress, h)
	if err != nil {
		return ResolvedAsset{}, err
	}
	return resolved[0], nil
}

// resolveGitHubAssets 解析 GitHub release 资源并下载所有匹配的唯一资产。
func resolveGitHubAssets(source string, patterns []string, downloadDir, githubToken, proxy string, retries int, progress io.Writer, h Hasher) ([]ResolvedAsset, error) {
	spec := strings.TrimPrefix(source, "github:")
	repoPart, tag, hasTag := strings.Cut(spec, "@")
	parts := strings.Split(repoPart, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid github source: %s", source)
	}
	if !validGitHubName(parts[0]) || !validGitHubName(parts[1]) {
		return nil, fmt.Errorf("invalid github source: %s", source)
	}
	for _, pattern := range patterns {
		if pattern == "" {
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
	if err := getJSON(api, &rel, githubHeaders(githubToken, true), proxy, retries, requestKind, tag); err != nil {
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
	resolved := make([]ResolvedAsset, 0, len(patterns))
	seen := map[string]ResolvedAsset{}
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		var matches []struct {
			Name               string
			BrowserDownloadURL string
		}
		for _, asset := range rel.Assets {
			if re.MatchString(asset.Name) && asset.BrowserDownloadURL != "" {
				matches = append(matches, struct {
					Name               string
					BrowserDownloadURL string
				}{Name: asset.Name, BrowserDownloadURL: asset.BrowserDownloadURL})
			}
		}
		if len(matches) > 1 {
			names := make([]string, 0, len(matches))
			for _, asset := range matches {
				names = append(names, asset.Name)
			}
			return nil, fmt.Errorf("multiple GitHub assets matched %q in %s/%s@%s: %s", pattern, parts[0], parts[1], releaseTag, strings.Join(names, ", "))
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no GitHub asset matched %q in %s/%s@%s", pattern, parts[0], parts[1], releaseTag)
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
		if err := download(asset.BrowserDownloadURL, target, githubHeaders(githubToken, false), proxy, retries, progress); err != nil {
			return nil, err
		}
		sum, err := h.SHA256File(target)
		if err != nil {
			return nil, err
		}
		resolvedAsset.Path = target
		resolvedAsset.SHA256 = sum
		resolved = append(resolved, resolvedAsset)
	}
	return resolved, nil
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
