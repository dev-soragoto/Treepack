package verify

import (
	"os"
	"path/filepath"
	"testing"

	"treepack/internal/manifest"
)

// TestRun 验证文件存在和不存在规则会生成正确的验证结果。
func TestRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "present.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "present-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	results := Run(manifest.VerifyConfig{
		Files:  []string{"present.txt", "missing.txt", "present-dir"},
		Dirs:   []string{"present-dir", "missing-dir", "present.txt"},
		Absent: []string{"gone.txt", "present.txt"},
	}, dir)
	if len(results) != 8 {
		t.Fatalf("got %d results", len(results))
	}
	if !results[0].OK || results[1].OK || results[2].OK || !results[3].OK || results[4].OK || results[5].OK || !results[6].OK || results[7].OK {
		t.Fatalf("unexpected verification results: %+v", results)
	}
	if results[2].Message != "required path is not a regular file" {
		t.Fatalf("unexpected file type message: %+v", results[2])
	}
	if results[5].Message != "required path is not a directory" {
		t.Fatalf("unexpected dir type message: %+v", results[5])
	}
}

// TestRunRejectsTraversal 验证验证规则中的路径穿越会失败。
func TestRunRejectsTraversal(t *testing.T) {
	results := Run(manifest.VerifyConfig{Files: []string{"../outside.txt"}, Dirs: []string{"../outside"}, Absent: []string{"../outside.txt"}}, t.TempDir())
	if len(results) != 3 || results[0].OK || results[1].OK || results[2].OK {
		t.Fatalf("expected traversal checks to fail: %+v", results)
	}
}
