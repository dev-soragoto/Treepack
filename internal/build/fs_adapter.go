package build

import (
	"treepack/internal/fsutil"
)

// fsAdapter adapts fsutil package functions to the consumer-defined source and ops interfaces.
type fsAdapter struct{}

// CopyFile 将单个普通文件复制到目标路径。
func (fsAdapter) CopyFile(src, dst string) error { return fsutil.CopyFile(src, dst) }

// CopyTreeContents 将目录内容合并复制到目标目录。
func (fsAdapter) CopyTreeContents(src, dst string) error { return fsutil.CopyTreeContents(src, dst) }

// CopyExact 按源路径类型精确复制文件或目录。
func (fsAdapter) CopyExact(src, dst string) error { return fsutil.CopyExact(src, dst) }

// Remove 删除路径，路径不存在时视为成功。
func (fsAdapter) Remove(path string) error { return fsutil.Remove(path) }

// EnsureParent 确保目标路径的父目录存在。
func (fsAdapter) EnsureParent(path string) error { return fsutil.EnsureParent(path) }

// SHA256File 计算文件的 SHA-256 十六进制摘要。
func (fsAdapter) SHA256File(path string) (string, error) { return sha256File(path) }
