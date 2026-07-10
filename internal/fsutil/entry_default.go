//go:build !windows

package fsutil

import "os"

// validateEntryPlatform 在非 Windows 平台保留平台相关文件类型校验钩子。
func validateEntryPlatform(path string, info os.FileInfo) error {
	return nil
}
