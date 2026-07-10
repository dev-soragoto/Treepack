package ops

import (
	"os"
	"path/filepath"
	"testing"

	"treepack/internal/fsutil"
)

type testFS struct{}

// CopyFile 实现测试文件系统适配器的对应接口。
func (testFS) CopyFile(src, dst string) error {
	return fsutil.CopyFile(src, dst)
}

// CopyTreeContents 实现测试文件系统适配器的对应接口。
func (testFS) CopyTreeContents(src, dst string) error {
	return fsutil.CopyTreeContents(src, dst)
}

// CopyExact 实现测试文件系统适配器的对应接口。
func (testFS) CopyExact(src, dst string) error {
	return fsutil.CopyExact(src, dst)
}

// Remove 实现测试文件系统适配器的对应接口。
func (testFS) Remove(path string) error {
	return fsutil.Remove(path)
}

// EnsureParent 实现测试文件系统适配器的对应接口。
func (testFS) EnsureParent(path string) error {
	return fsutil.EnsureParent(path)
}

// TestCpFileToFile 验证对应场景的行为。
func TestCpFileToFile(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "src.txt"), "source")

	runOK(t, work, OperationConfig{Op: "cp", From: "src.txt", To: "copied/dst.txt"})

	assertFileContent(t, filepath.Join(work, "copied", "dst.txt"), "source")
}

// TestCpFileToDirectory 验证对应场景的行为。
func TestCpFileToDirectory(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "src.txt"), "source")
	if err := os.MkdirAll(filepath.Join(work, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}

	runOK(t, work, OperationConfig{Op: "cp", From: "src.txt", To: "dest"})

	assertFileContent(t, filepath.Join(work, "dest", "src.txt"), "source")
}

// TestCpDirectoryToNewDirectory 验证对应场景的行为。
func TestCpDirectoryToNewDirectory(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "srcdir", "child.txt"), "child")

	runOK(t, work, OperationConfig{Op: "cp", From: "srcdir", To: "dest"})

	assertFileContent(t, filepath.Join(work, "dest", "child.txt"), "child")
}

// TestCpDirectoryToExistingDirectory 验证对应场景的行为。
func TestCpDirectoryToExistingDirectory(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "srcdir", "child.txt"), "child")
	mustWrite(t, filepath.Join(work, "dest", "keep.txt"), "keep")

	runOK(t, work, OperationConfig{Op: "cp", From: "srcdir", To: "dest"})

	assertFileContent(t, filepath.Join(work, "dest", "keep.txt"), "keep")
	assertFileContent(t, filepath.Join(work, "dest", "srcdir", "child.txt"), "child")
}

// TestCpDirectoryDotCopiesContents 验证对应场景的行为。
func TestCpDirectoryDotCopiesContents(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "srcdir", "child.txt"), "child")
	mustWrite(t, filepath.Join(work, "dest", "keep.txt"), "keep")

	runOK(t, work, OperationConfig{Op: "cp", From: "srcdir/.", To: "dest"})

	assertFileContent(t, filepath.Join(work, "dest", "keep.txt"), "keep")
	assertFileContent(t, filepath.Join(work, "dest", "child.txt"), "child")
	assertMissing(t, filepath.Join(work, "dest", "srcdir"))
}

// TestCpOverwritesMergesAndHandlesTypeConflicts 验证对应场景的行为。
func TestCpOverwritesMergesAndHandlesTypeConflicts(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "new.txt"), "new")
	mustWrite(t, filepath.Join(work, "dest", "old.txt"), "old")
	mustWrite(t, filepath.Join(work, "srcdir", "merge", "fresh.txt"), "fresh")
	mustWrite(t, filepath.Join(work, "destdir", "merge", "keep.txt"), "keep")
	mustWrite(t, filepath.Join(work, "dirsrc", "child.txt"), "child")
	mustWrite(t, filepath.Join(work, "filedest"), "file")
	mustWrite(t, filepath.Join(work, "filesrc"), "file replaces dir")
	if err := os.MkdirAll(filepath.Join(work, "dirdest"), 0o755); err != nil {
		t.Fatal(err)
	}

	runOK(t, work, OperationConfig{Op: "cp", From: "new.txt", To: "dest/old.txt"})
	runOK(t, work, OperationConfig{Op: "cp", From: "srcdir/.", To: "destdir"})
	runOK(t, work, OperationConfig{Op: "cp", From: "dirsrc", To: "filedest"})
	runOK(t, work, OperationConfig{Op: "cp", From: "filesrc", To: "dirdest"})

	assertFileContent(t, filepath.Join(work, "dest", "old.txt"), "new")
	assertFileContent(t, filepath.Join(work, "destdir", "merge", "keep.txt"), "keep")
	assertFileContent(t, filepath.Join(work, "destdir", "merge", "fresh.txt"), "fresh")
	assertFileContent(t, filepath.Join(work, "filedest", "child.txt"), "child")
	assertFileContent(t, filepath.Join(work, "dirdest", "filesrc"), "file replaces dir")
}

// TestCpRegexCopiesDirectMatches 验证对应场景的行为。
func TestCpRegexCopiesDirectMatches(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "search", "test.one"), "one")
	mustWrite(t, filepath.Join(work, "search", "testdir", "child.txt"), "child")
	mustWrite(t, filepath.Join(work, "search", "other.txt"), "other")

	runOK(t, work, OperationConfig{Op: "cp_regex", From: "search", Regex: `^test`, To: "save"})

	assertFileContent(t, filepath.Join(work, "save", "test.one"), "one")
	assertFileContent(t, filepath.Join(work, "save", "testdir", "child.txt"), "child")
	assertMissing(t, filepath.Join(work, "save", "other.txt"))
}

// TestCpRegexDoesNotMatchDeepEntries 验证对应场景的行为。
func TestCpRegexDoesNotMatchDeepEntries(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "search", "deep", "test.txt"), "deep")

	result := Run(OperationConfig{Op: "cp_regex", From: "search", Regex: `^test\.txt$`, To: "save"}, work, testFS{})
	if result.OK {
		t.Fatalf("expected cp_regex without direct matches to fail")
	}
	assertMissing(t, filepath.Join(work, "save", "test.txt"))
}

// TestCpRegexRequiresDirectoryAndMatch 验证对应场景的行为。
func TestCpRegexRequiresDirectoryAndMatch(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "file.txt"), "file")
	if err := os.MkdirAll(filepath.Join(work, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, op := range []OperationConfig{
		{Op: "cp_regex", From: "file.txt", Regex: `.*`, To: "save"},
		{Op: "cp_regex", From: "empty", Regex: `nomatch`, To: "save"},
	} {
		if result := Run(op, work, testFS{}); result.OK {
			t.Fatalf("expected failure: %+v", op)
		}
	}
}

// TestCpRejectsOverlappingPaths 验证对应场景的行为。
func TestCpRejectsOverlappingPaths(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "same.txt"), "keep")
	mustWrite(t, filepath.Join(work, "dir", "child.txt"), "child")

	for _, op := range []OperationConfig{
		{Op: "cp", From: "same.txt", To: "same.txt"},
		{Op: "cp", From: "dir", To: "dir/sub"},
		{Op: "cp", From: "dir/.", To: "dir/sub"},
		{Op: "cp_regex", From: "dir", Regex: `.*`, To: "dir/sub"},
	} {
		if result := Run(op, work, testFS{}); result.OK {
			t.Fatalf("expected overlap copy to fail: %+v", op)
		}
	}
	assertFileContent(t, filepath.Join(work, "same.txt"), "keep")
}

// TestCpRegexRejectsSymlinkOverlap 验证对应场景的行为。
func TestCpRegexRejectsSymlinkOverlap(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("symlink creation on Windows may require privileges")
	}
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "src", "file.txt"), "file")
	if err := os.Symlink(filepath.Join(work, "src"), filepath.Join(work, "link")); err != nil {
		t.Fatal(err)
	}
	result := Run(OperationConfig{Op: "cp_regex", From: "src", Regex: `.*`, To: "link"}, work, testFS{})
	if result.OK {
		t.Fatal("expected symlink overlap to fail")
	}
}

// TestRmRemovesFilesDirectoriesAndMissingPaths 验证对应场景的行为。
func TestRmRemovesFilesDirectoriesAndMissingPaths(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "file.txt"), "file")
	mustWrite(t, filepath.Join(work, "dir", "child.txt"), "child")

	runOK(t, work, OperationConfig{Op: "rm", Path: "file.txt"})
	runOK(t, work, OperationConfig{Op: "rm", Path: "dir"})
	runOK(t, work, OperationConfig{Op: "rm", Path: "missing"})

	assertMissing(t, filepath.Join(work, "file.txt"))
	assertMissing(t, filepath.Join(work, "dir"))
}

// TestTouchCreatesMarkerWithoutTruncating 验证对应场景的行为。
func TestTouchCreatesMarkerWithoutTruncating(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "marker"), "keep")

	runOK(t, work, OperationConfig{Op: "touch", Path: "marker"})
	runOK(t, work, OperationConfig{Op: "touch", Path: "nested/READY"})

	assertFileContent(t, filepath.Join(work, "marker"), "keep")
	assertExists(t, filepath.Join(work, "nested", "READY"))
}

// TestRunRejectsPathTraversal 验证对应场景的行为。
func TestRunRejectsPathTraversal(t *testing.T) {
	work := t.TempDir()
	mustWrite(t, filepath.Join(work, "inside.txt"), "inside")
	for _, op := range []OperationConfig{
		{Op: "touch", Path: "../outside.txt"},
		{Op: "rm", Path: "../outside.txt"},
		{Op: "cp", From: "../outside.txt", To: "copied.txt"},
		{Op: "cp", From: "inside.txt", To: "../outside.txt"},
		{Op: "cp_regex", From: "../outside", Regex: `.*`, To: "copied"},
		{Op: "cp_regex", From: ".", Regex: `.*`, To: "../outside"},
	} {
		if result := Run(op, work, testFS{}); result.OK {
			t.Fatalf("expected traversal op to fail: %+v", op)
		}
	}
	assertMissing(t, filepath.Join(filepath.Dir(work), "outside.txt"))
}

// runOK 执行操作并断言操作成功。
func runOK(t *testing.T, work string, op OperationConfig) {
	t.Helper()
	if result := Run(op, work, testFS{}); !result.OK {
		t.Fatalf("%s failed: %s", result.Label, result.Message)
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

// assertExists 断言测试期望，失败时终止当前测试。
func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

// assertMissing 断言测试期望，失败时终止当前测试。
func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent", path)
	}
}

// assertFileContent 断言测试期望，失败时终止当前测试。
func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}
