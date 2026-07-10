//go:build windows

package fsutil

import (
	"fmt"
	"os"
)

// validateEntryPlatform 在 Windows 平台拒绝符号链接和 junction 等重解析入口。
func validateEntryPlatform(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsupported file type: %s", path)
	}
	return nil
}
