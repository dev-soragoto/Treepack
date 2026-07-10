package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type EntryKind int

const (
	EntryFile EntryKind = iota
	EntryDir
)

// ValidateEntry 校验路径指向受支持的文件系统入口并返回入口类型。
func ValidateEntry(path string) (os.FileInfo, EntryKind, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if err := validateEntryPlatform(path, info); err != nil {
		return nil, 0, err
	}
	mode := info.Mode()
	switch {
	case mode.IsRegular():
		return info, EntryFile, nil
	case mode.IsDir() && mode&os.ModeType == os.ModeDir:
		return info, EntryDir, nil
	default:
		return nil, 0, fmt.Errorf("unsupported file type: %s", path)
	}
}

// CopyFile 复制单个普通文件，并拒绝源目标路径重叠。
func CopyFile(src, dst string) error {
	info, kind, err := ValidateEntry(src)
	if err != nil {
		return err
	}
	if kind != EntryFile {
		return fmt.Errorf("%s is not a regular file", src)
	}
	if err := RejectCopyOverlap(src, dst); err != nil {
		return err
	}
	return copyFileValidated(src, dst, info)
}

// CopyTreeContents 将源目录中的内容复制并合并到目标目录。
func CopyTreeContents(src, dst string) error {
	if _, kind, err := ValidateEntry(src); err != nil {
		return err
	} else if kind != EntryDir {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := RejectCopyOverlap(src, dst); err != nil {
		return err
	}
	if err := replaceDestination(dst, EntryDir); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if err := CopyExact(from, to); err != nil {
			return err
		}
	}
	return nil
}

// CopyExact 按源入口类型将文件或完整目录复制到精确目标路径。
func CopyExact(src, dst string) error {
	if err := RejectCopyOverlap(src, dst); err != nil {
		return err
	}
	info, kind, err := ValidateEntry(src)
	if err != nil {
		return err
	}
	if kind == EntryFile {
		return copyFileValidated(src, dst, info)
	}
	return copyDirExact(src, dst)
}

// Remove 删除文件或目录，目标不存在时不返回错误。
func Remove(path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// EnsureParent 确保给定文件路径的父目录存在。
func EnsureParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

// copyDirExact 将完整目录树复制到指定目标路径。
func copyDirExact(src, dst string) error {
	if err := replaceDestination(dst, EntryDir); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := CopyExact(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFileValidated 在已校验源文件信息的前提下复制文件内容和权限。
func copyFileValidated(src, dst string, info os.FileInfo) error {
	if err := replaceDestination(dst, EntryFile); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Chmod(dst, info.Mode().Perm())
	return nil
}

// replaceDestination 按期望类型清理目标路径上的冲突入口。
func replaceDestination(path string, want EntryKind) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if want == EntryDir && info.IsDir() && info.Mode()&os.ModeType == os.ModeDir {
		return nil
	}
	return Remove(path)
}

// RejectCopyOverlap 拒绝源路径与目标路径存在包含或相同关系的复制。
func RejectCopyOverlap(src, dst string) error {
	overlap, err := Overlap(src, dst)
	if err != nil {
		return err
	}
	if overlap {
		return fmt.Errorf("copy source and destination cannot overlap: %s / %s", src, dst)
	}
	return nil
}
