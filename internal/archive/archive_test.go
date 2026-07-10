package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMakeZipCreatesArchive 验证对应场景的行为。
func TestMakeZipCreatesArchive(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "dir", "file.txt"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "out.zip")
	if err := MakeZip(source, archivePath); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got := archiveEntryNames(reader.File)
	want := []string{"dir/", "dir/file.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

// TestMakeZipIncludesEmptyDirectories 验证对应场景的行为。
func TestMakeZipIncludesEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "empty", "layout"), 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "out.zip")
	if err := MakeZip(source, archivePath); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got := archiveEntryNames(reader.File)
	want := []string{"empty/", "empty/layout/"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

// TestMakeZipRejectsExistingDirectory 验证对应场景的行为。
func TestMakeZipRejectsExistingDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "out.zip")
	if err := os.MkdirAll(archivePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := MakeZip(source, archivePath); err == nil {
		t.Fatal("expected existing directory archive path to fail")
	}
}

// TestMakeZipRejectsSymlink 验证对应场景的行为。
func TestMakeZipRejectsSymlink(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("symlink creation on Windows may require privileges")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "target.txt"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "target.txt"), filepath.Join(source, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := MakeZip(source, filepath.Join(root, "out.zip")); err == nil {
		t.Fatal("expected symlink in source tree to fail")
	}
}

// TestWriteZipReturnsWriterCloseError 验证对应场景的行为。
func TestWriteZipReturnsWriterCloseError(t *testing.T) {
	source := t.TempDir()
	writer := zip.NewWriter(errorWriter{})
	err := writeZip(source, writer)
	if err == nil || !strings.Contains(err.Error(), "zip close failed") {
		t.Fatalf("expected zip close error, got %v", err)
	}
}

// TestMakeZipReturnsOutputCloseError 验证对应场景的行为。
func TestMakeZipReturnsOutputCloseError(t *testing.T) {
	source := t.TempDir()
	closeErr := errors.New("output close failed")
	oldCreate := createArchiveFile
	createArchiveFile = func(dir, pattern string) (archiveOutput, error) {
		return &closeFailWriter{name: filepath.Join(dir, "tmp.zip"), err: closeErr}, nil
	}
	defer func() { createArchiveFile = oldCreate }()
	err := MakeZip(source, filepath.Join(t.TempDir(), "out.zip"))
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected output close error, got %v", err)
	}
}

// TestMakeZipFailureDoesNotReplaceExistingArchive 验证对应场景的行为。
func TestMakeZipFailureDoesNotReplaceExistingArchive(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "out.zip")
	if err := os.WriteFile(archivePath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldCreate := createArchiveFile
	createArchiveFile = func(dir, pattern string) (archiveOutput, error) {
		return &closeFailWriter{name: filepath.Join(dir, "tmp.zip"), err: errors.New("close failed")}, nil
	}
	defer func() { createArchiveFile = oldCreate }()
	if err := MakeZip(source, archivePath); err == nil {
		t.Fatal("expected archive creation to fail")
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("existing archive was replaced: %q", data)
	}
}

// TestExtractZipRejectsDuplicateEntries 验证对应场景的行为。
func TestExtractZipRejectsDuplicateEntries(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "dup.zip")
	writeZipEntries(t, zipPath, []zipEntry{
		{name: "a.txt", body: "one"},
		{name: "a.txt", body: "two"},
	})
	if err := ExtractZip(zipPath, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected duplicate entry to fail")
	}
}

// TestExtractZipRejectsSymlinkEntries 验证对应场景的行为。
func TestExtractZipRejectsSymlinkEntries(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "link.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	header := &zip.FileHeader{Name: "link"}
	header.SetMode(os.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("target")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZip(zipPath, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected symlink entry to fail")
	}
}

// TestExtractZipRejectsSpecialEntries 验证对应场景的行为。
func TestExtractZipRejectsSpecialEntries(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "special.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	header := &zip.FileHeader{Name: "pipe"}
	header.SetMode(os.ModeNamedPipe | 0o644)
	if _, err := zw.CreateHeader(header); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZip(zipPath, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected special entry to fail")
	}
}

// TestExtractZipRejectsEscapingEntry 验证对应场景的行为。
func TestExtractZipRejectsEscapingEntry(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "escape.zip")
	writeZipEntries(t, zipPath, []zipEntry{{name: "../outside.txt", body: "bad"}})
	if err := ExtractZip(zipPath, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected escaping entry to fail")
	}
}

// TestExtractZipPreservesExactMode 验证 ZIP 解压使用 entry 原始普通权限，不会扩张执行位。
func TestExtractZipPreservesExactMode(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("Windows does not preserve Unix execute bits")
	}
	root := t.TempDir()
	zipPath := filepath.Join(root, "perms.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	header := &zip.FileHeader{Name: "script"}
	header.SetMode(0o700)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("body")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(root, "out")
	if err := ExtractZip(zipPath, outputDir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(outputDir, "script"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("mode = %o, want 700", got)
	}
}

type zipEntry struct {
	name string
	body string
}

type errorWriter struct{}

// Write 模拟写入失败的测试 writer。
func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("zip close failed")
}

type closeFailWriter struct {
	bytes.Buffer
	name string
	err  error
}

// Close 实现关闭失败 writer 的对应接口。
func (w *closeFailWriter) Close() error {
	return w.err
}

// Name 实现关闭失败 writer 的对应接口。
func (w *closeFailWriter) Name() string {
	return w.name
}

// archiveEntryNames 提取归档条目名称用于断言。
func archiveEntryNames(files []*zip.File) []string {
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.Name)
	}
	return names
}

// writeZipEntries 创建测试所需的 zip 归档内容。
func writeZipEntries(t *testing.T, path string, entries []zipEntry) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for _, entry := range entries {
		w, err := zw.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
