package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCopyExactOverwritesTypeConflicts 验证对应场景的行为。
func TestCopyExactOverwritesTypeConflicts(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "file-src"), "file")
	mustWrite(t, filepath.Join(root, "dir-src", "child.txt"), "child")
	mustWrite(t, filepath.Join(root, "file-dst"), "old")
	if err := os.MkdirAll(filepath.Join(root, "dir-dst"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := CopyExact(filepath.Join(root, "dir-src"), filepath.Join(root, "file-dst")); err != nil {
		t.Fatal(err)
	}
	if err := CopyExact(filepath.Join(root, "file-src"), filepath.Join(root, "dir-dst")); err != nil {
		t.Fatal(err)
	}

	assertFile(t, filepath.Join(root, "file-dst", "child.txt"), "child")
	assertFile(t, filepath.Join(root, "dir-dst"), "file")
}

// TestCopyTreeContentsMergesDirectories 验证对应场景的行为。
func TestCopyTreeContentsMergesDirectories(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "merge", "new.txt"), "new")
	mustWrite(t, filepath.Join(root, "dst", "merge", "keep.txt"), "keep")

	if err := CopyTreeContents(filepath.Join(root, "src"), filepath.Join(root, "dst")); err != nil {
		t.Fatal(err)
	}

	assertFile(t, filepath.Join(root, "dst", "merge", "new.txt"), "new")
	assertFile(t, filepath.Join(root, "dst", "merge", "keep.txt"), "keep")
}

// TestCopyRejectsSymlink 验证对应场景的行为。
func TestCopyRejectsSymlink(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("symlink creation on Windows may require privileges")
	}
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "target.txt"), "target")
	if err := os.Symlink(filepath.Join(root, "target.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(filepath.Join(root, "link.txt"), filepath.Join(root, "out.txt")); err == nil {
		t.Fatal("expected symlink source to fail")
	}
}

// TestCopyRejectsOverlap 验证对应场景的行为。
func TestCopyRejectsOverlap(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "file.txt"), "file")
	if err := CopyTreeContents(filepath.Join(root, "src"), filepath.Join(root, "src", "nested")); err == nil {
		t.Fatal("expected overlap copy to fail")
	}
}

// mustWrite 写入测试文件，失败时终止当前测试。
func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertFile 断言测试期望，失败时终止当前测试。
func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}
