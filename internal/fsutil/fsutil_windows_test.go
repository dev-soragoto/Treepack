//go:build windows

package fsutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestValidateEntryRejectsJunction 验证统一文件类型入口会拒绝 Windows junction。
func TestValidateEntryRejectsJunction(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "junction")
	createJunctionFSUtil(t, junction, target)

	if _, _, err := ValidateEntry(junction); err == nil {
		t.Fatal("expected junction to fail")
	}
}

// TestCopyTreeRejectsJunction 验证目录复制遇到 junction 时失败，避免复制越过文件树边界。
func TestCopyTreeRejectsJunction(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	createJunctionFSUtil(t, filepath.Join(src, "junction"), target)

	if err := CopyTreeContents(src, filepath.Join(root, "dst")); err == nil {
		t.Fatal("expected junction in source tree to fail")
	}
}

// createJunctionFSUtil 创建 Windows junction 供平台相关测试使用。
func createJunctionFSUtil(t *testing.T, link, target string) {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, output)
	}
}
