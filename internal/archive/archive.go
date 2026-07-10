package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"treepack/internal/fsutil"
)

type archiveOutput interface {
	io.Writer
	io.Closer
	Name() string
}

var createArchiveFile = func(dir, pattern string) (archiveOutput, error) {
	return os.CreateTemp(dir, pattern)
}

// ExtractZip 从 zip 归档中提取普通文件和真实目录到输出目录，并拒绝符号链接、特殊文件和越界路径。
func ExtractZip(archivePath, outputDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	cleanOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, file := range reader.File {
		mode := file.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("zip symlink entries are not supported: %s", file.Name)
		}
		if !mode.IsDir() && !mode.IsRegular() {
			return fmt.Errorf("zip special file entries are not supported: %s", file.Name)
		}
		cleanName := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Name)))
		if cleanName == "." {
			continue
		}
		key := cleanName
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return fmt.Errorf("zip contains duplicate entry: %s", file.Name)
		}
		seen[key] = true
		target := filepath.Join(outputDir, filepath.FromSlash(file.Name))
		cleanTarget, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if cleanTarget != cleanOutput && !strings.HasPrefix(cleanTarget, cleanOutput+string(filepath.Separator)) {
			return fmt.Errorf("zip entry escapes output directory: %s", file.Name)
		}
		if mode.IsDir() {
			if info, err := os.Lstat(target); err == nil && !isRealDir(info) {
				if err := fsutil.Remove(target); err != nil {
					return err
				}
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := fsutil.Remove(target); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		perm := file.FileInfo().Mode().Perm()
		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		_ = os.Chmod(target, perm)
	}
	return nil
}

// MakeZip 从源目录创建 zip 归档，并通过临时文件保证失败时不替换既有归档。
func MakeZip(sourceDir, archivePath string) error {
	archiveDir := filepath.Dir(archivePath)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(archivePath); err == nil && info.IsDir() {
		return fmt.Errorf("archive path is an existing directory: %s", archivePath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	out, err := createArchiveFile(archiveDir, ".treepack-*.zip")
	if err != nil {
		return err
	}
	tmpPath := out.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	writer := zip.NewWriter(out)
	writeErr := writeZip(sourceDir, writer)
	closeErr := out.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		return err
	}
	committed = true
	return nil
}

// writeZip 按稳定顺序把源目录中的普通文件和真实目录写入 zip writer。
func writeZip(sourceDir string, writer *zip.Writer) error {
	var entries []archiveEntry
	if err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == sourceDir {
			return nil
		}
		_, kind, err := fsutil.ValidateEntry(path)
		if err != nil {
			return err
		}
		if kind == fsutil.EntryFile || kind == fsutil.EntryDir {
			rel, err := filepath.Rel(sourceDir, path)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(rel)
			if kind == fsutil.EntryDir {
				name += "/"
			}
			entries = append(entries, archiveEntry{path: path, name: name, kind: kind})
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})
	for _, entry := range entries {
		info, _, err := fsutil.ValidateEntry(entry.path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = entry.name
		if entry.kind == fsutil.EntryDir {
			header.Method = zip.Store
			if _, err := writer.CreateHeader(header); err != nil {
				return err
			}
			continue
		}
		header.Method = zip.Deflate
		dst, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(entry.path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, in)
		in.Close()
		if copyErr != nil {
			writer.Close()
			return copyErr
		}
	}
	return writer.Close()
}

// isRealDir 判断文件信息是否表示真实目录而不是其他目录类特殊项。
func isRealDir(info os.FileInfo) bool {
	return info.IsDir() && info.Mode()&os.ModeType == os.ModeDir
}

type archiveEntry struct {
	path string
	name string
	kind fsutil.EntryKind
}
