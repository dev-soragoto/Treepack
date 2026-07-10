//go:build windows

package archive

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMakeZipRejectsJunction 验证创建 ZIP 时拒绝 source tree 中的 junction。
func TestMakeZipRejectsJunction(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	createJunctionArchive(t, filepath.Join(source, "junction"), target)

	if err := MakeZip(source, filepath.Join(root, "out.zip"), Options{}); err == nil {
		t.Fatal("expected junction in source tree to fail")
	}
}

// createJunctionArchive 创建 Windows junction 供平台相关测试使用。
func createJunctionArchive(t *testing.T, link, target string) {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, output)
	}
}
