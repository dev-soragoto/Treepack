package source

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"treepack/internal/fsutil"
)

// resolveFile 从本地 file: 来源解析文件或目录并复制到下载目录。
func resolveFile(source, pattern, root, downloadDir string, h Hasher) (ResolvedAsset, error) {
	raw := strings.TrimPrefix(source, "file:")
	src := filepath.FromSlash(raw)
	if filepath.IsAbs(src) {
		if err := fsutil.CheckUnder(root, src); err != nil {
			return ResolvedAsset{}, err
		}
	} else {
		var err error
		src, err = fsutil.ResolveUnder(root, raw)
		if err != nil {
			return ResolvedAsset{}, err
		}
	}
	_, kind, err := fsutil.ValidateEntry(src)
	if err != nil {
		if os.IsNotExist(err) {
			return ResolvedAsset{}, fmt.Errorf("local source does not exist: %s", src)
		}
		return ResolvedAsset{}, fmt.Errorf("invalid local source %s: %w", src, err)
	}
	if kind == fsutil.EntryDir {
		if pattern == "" {
			return copyLocalAsset(source, root, src, downloadDir, kind, h)
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return ResolvedAsset{}, err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return ResolvedAsset{}, err
		}
		var matches []string
		for _, entry := range entries {
			if !re.MatchString(entry.Name()) {
				continue
			}
			path := filepath.Join(src, entry.Name())
			_, kind, err := fsutil.ValidateEntry(path)
			if err != nil {
				return ResolvedAsset{}, err
			}
			if kind == fsutil.EntryFile || kind == fsutil.EntryDir {
				matches = append(matches, path)
			}
		}
		sort.Strings(matches)
		if len(matches) == 0 {
			return ResolvedAsset{}, fmt.Errorf("no local asset matched %q in %s", pattern, src)
		}
		if len(matches) > 1 {
			return ResolvedAsset{}, fmt.Errorf("multiple local assets matched %q in %s: %s", pattern, src, strings.Join(baseNames(matches), ", "))
		}
		src = matches[0]
		_, kind, err = fsutil.ValidateEntry(src)
		if err != nil {
			return ResolvedAsset{}, err
		}
	} else if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return ResolvedAsset{}, err
		}
		name := filepath.Base(src)
		if !re.MatchString(name) {
			return ResolvedAsset{}, fmt.Errorf("local filename %q does not match asset pattern %q", name, pattern)
		}
	}
	return copyLocalAsset(source, root, src, downloadDir, kind, h)
}

// copyLocalAsset copies a validated local file or directory into the download area.
func copyLocalAsset(source, root, src, downloadDir string, kind fsutil.EntryKind, h Hasher) (ResolvedAsset, error) {
	target, err := fsutil.ResolveUnder(downloadDir, filepath.Base(src))
	if err != nil {
		return ResolvedAsset{}, err
	}
	if overlap, err := fsutil.Overlap(src, target); err != nil {
		return ResolvedAsset{}, err
	} else if overlap {
		return ResolvedAsset{}, fmt.Errorf("local source and download target cannot overlap: %s / %s", src, target)
	}
	if err := h.CopyExact(src, target); err != nil {
		return ResolvedAsset{}, err
	}
	assetKind := "file"
	var sum string
	if kind == fsutil.EntryDir {
		assetKind = "dir"
	} else {
		sum, err = h.SHA256File(target)
		if err != nil {
			return ResolvedAsset{}, err
		}
	}
	return ResolvedAsset{Source: source, Requested: "local", Resolved: "local", AssetName: filepath.Base(src), Kind: assetKind, URL: localFileRef(root, src), Path: target, SHA256: sum}, nil
}

// baseNames 返回路径列表对应的基础文件名列表。
func baseNames(paths []string) []string {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, filepath.Base(path))
	}
	return names
}

// localFileRef 将本地文件路径转换为报告中使用的相对 file: 引用。
func localFileRef(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		rel = filepath.Base(path)
	}
	return "file:" + filepath.ToSlash(rel)
}
