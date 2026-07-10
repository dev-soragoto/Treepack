package fsutil

import (
	"path/filepath"
	"testing"
)

// TestResolveUnderRejectsEscapes 验证路径解析会拒绝父级穿越和绝对路径。
func TestResolveUnderRejectsEscapes(t *testing.T) {
	base := t.TempDir()
	if _, err := ResolveUnder(base, "../outside.txt"); err == nil {
		t.Fatal("expected parent traversal to be rejected")
	}
	if _, err := ResolveUnder(base, filepath.Join(base, "absolute.txt")); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
	path, err := ResolveUnder(base, "nested/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckUnder(base, path); err != nil {
		t.Fatal(err)
	}
}
