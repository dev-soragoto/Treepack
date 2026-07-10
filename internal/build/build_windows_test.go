//go:build windows

package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildRejectsJunctionSource 验证 paths.source 是 junction 时会被拒绝。
func TestBuildRejectsJunctionSource(t *testing.T) {
	root := t.TempDir()
	realSource := filepath.Join(root, "real-src")
	if err := os.MkdirAll(realSource, 0o755); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "junction-src")
	createJunctionBuild(t, junction, realSource)

	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Junction Source"
[paths]
source = "SOURCE"
output = "OUTPUT"
`)
	body, err := os.ReadFile(filepath.Join(root, "kit.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(body), "SOURCE", filepath.ToSlash(junction)), "OUTPUT", filepath.ToSlash(filepath.Join(root, "out")))
	if err := os.WriteFile(filepath.Join(root, "kit.toml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err == nil {
		t.Fatal("expected junction paths.source to fail")
	}
}

// createJunctionBuild 创建 Windows junction 供平台相关测试使用。
func createJunctionBuild(t *testing.T, link, target string) {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, output)
	}
}
