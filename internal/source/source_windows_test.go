//go:build windows

package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestResolveFileRejectsJunction 验证 local file: source 指向 junction 时会被拒绝。
func TestResolveFileRejectsJunction(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	createJunctionSource(t, filepath.Join(root, "junction"), target)

	if _, err := resolveForTest(ResolveRequest{
		Source:      "file:junction",
		Assets:      []AssetRequest{{Pattern: ".*"}},
		Root:        root,
		DownloadDir: filepath.Join(root, "downloads"),
	}); err == nil {
		t.Fatal("expected junction local source to fail")
	}
}

// createJunctionSource 创建 Windows junction 供平台相关测试使用。
func createJunctionSource(t *testing.T, link, target string) {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, output)
	}
}
