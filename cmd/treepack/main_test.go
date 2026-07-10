package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionFlags 验证长短版本参数都会输出源码默认开发版本号。
func TestVersionFlags(t *testing.T) {
	for _, flag := range []string{"-v", "--version"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{flag}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s returned %d, stderr: %s", flag, code, stderr.String())
		}
		if stdout.String() != "treepack dev\n" {
			t.Fatalf("%s stdout = %q", flag, stdout.String())
		}
	}
}

// TestHelpFlags 验证对应场景的行为。
func TestHelpFlags(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{flag}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s returned %d, stderr: %s", flag, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%s stderr = %q", flag, stderr.String())
		}
		assertHelpText(t, stdout.String())
	}
}

// TestBuildSubcommandIsUnexpectedArgument 验证不存在的 build 子命令会作为多余参数报错。
func TestBuildSubcommandIsUnexpectedArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"build"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unexpected argument: build") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	assertHelpText(t, stderr.String())
}

// TestMissingManifestPrintsUsage 验证 manifest 不存在时会同时输出错误和用法。
func TestMissingManifestPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", filepath.Join(t.TempDir(), "missing.toml")}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "cannot read manifest:") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	assertHelpText(t, stderr.String())
}

// TestShortConfigSourceOutputAndProxyFlags 验证 config、source、output 和 proxy 的短参数可以驱动构建。
func TestShortConfigSourceOutputAndProxyFlags(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMainTestFile(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "CLI Test"
[build]
archive = "cli-test.zip"
`)
	var stdout, stderr bytes.Buffer
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	code := run([]string{"-c", "kit.toml", "-s", sourceDir, "-o", outputDir, "-p", "socks5://127.0.0.1:7890"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "built CLI Test into "+outputDir) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "cli-test.zip")); err != nil {
		t.Fatalf("expected archive: %v", err)
	}
}

// TestManifestWithoutArchive 验证未声明 archive 时会跳过 zip 并保留输出目录。
func TestManifestWithoutArchive(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMainTestFile(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Directory Only"
[paths]
source = "SRC"
output = "OUT"
`)
	rewriteMainTestFile(t, filepath.Join(root, "kit.toml"), map[string]string{
		"SRC": filepath.ToSlash(sourceDir),
		"OUT": filepath.ToSlash(filepath.Join(root, "out")),
	})
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", filepath.Join(root, "kit.toml")}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "built Directory Only into "+filepath.Join(root, "out")) {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "out", "BUILD_INFO.txt")); err != nil {
		t.Fatalf("expected build info: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "directory-only.zip")); !os.IsNotExist(err) {
		t.Fatal("archive should not be created")
	}
}

// TestRawArchiveFlagIncludesMetadata 验证 --raw-archive 会保留默认会被过滤的归档元数据。
func TestRawArchiveFlagIncludesMetadata(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	writeMainTestFile(t, filepath.Join(sourceDir, "resources", ".DS_Store"), "ds")
	writeMainTestFile(t, filepath.Join(sourceDir, "resources", "__MACOSX", "metadata.txt"), "metadata")
	writeMainTestFile(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Raw CLI"
[paths]
source = "SRC"
output = "OUT"
[build]
archive = "raw-cli.zip"
[resources]
copy = "resources"
`)
	rewriteMainTestFile(t, filepath.Join(root, "kit.toml"), map[string]string{
		"SRC": filepath.ToSlash(sourceDir),
		"OUT": filepath.ToSlash(outputDir),
	})
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", filepath.Join(root, "kit.toml"), "--raw-archive"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr.String())
	}
	seen := mainZipEntrySet(t, filepath.Join(root, "raw-cli.zip"))
	for _, name := range []string{".DS_Store", "__MACOSX/", "__MACOSX/metadata.txt"} {
		if !seen[name] {
			t.Fatalf("raw archive missing metadata %s: %+v", name, seen)
		}
	}
}

// TestUnknownFlagReturnsUsageError 验证未知参数会返回参数错误。
func TestUnknownFlagReturnsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--unknown-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -unknown-flag") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

// TestKeepWorkFlagPreservesRunDir 验证 --keep-work 会保留 work run dir。
func TestKeepWorkFlagPreservesRunDir(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMainTestFile(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Keep Work"
[paths]
source = "SRC"
output = "OUT"
work = "WORK"
`)
	rewriteMainTestFile(t, filepath.Join(root, "kit.toml"), map[string]string{
		"SRC":  filepath.ToSlash(sourceDir),
		"OUT":  filepath.ToSlash(outputDir),
		"WORK": filepath.ToSlash(workDir),
	})
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", filepath.Join(root, "kit.toml"), "--keep-work"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr.String())
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected kept work run dir")
	}
}

// TestInvalidProxyReturnsUsageError 验证非法代理参数会返回参数错误和用法。
func TestInvalidProxyReturnsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--proxy", "127.0.0.1:7890"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "invalid proxy") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	assertHelpText(t, stderr.String())
}

// TestInvalidDownloadRetriesReturnsUsageError 验证对应场景的行为。
func TestInvalidDownloadRetriesReturnsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--download-retries", "0"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "invalid download retries") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	assertHelpText(t, stderr.String())
}

// writeMainTestFile 写入测试文件，失败时终止当前测试。
func writeMainTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rewriteMainTestFile 按替换表重写测试文件内容。
func rewriteMainTestFile(t *testing.T, path string, replacements map[string]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for old, newValue := range replacements {
		body = strings.ReplaceAll(body, old, newValue)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertHelpText 断言测试期望，失败时终止当前测试。
func assertHelpText(t *testing.T, body string) {
	t.Helper()
	for _, want := range []string{
		"Usage:",
		"treepack [-c kit.toml]",
		"Options:",
		"-c, --config kit.toml",
		"-s, --source DIR",
		"-o, --output DIR",
		"--work-dir DIR",
		"--keep-work",
		"--raw-archive",
		"-p, --proxy URL",
		"--download-retries N",
		"--github-token TOKEN",
		"-v, --version",
		"-h, --help",
		"Environment:",
		"GITHUB_TOKEN",
		"GH_TOKEN",
		"socks5h",
		"Path rules:",
		"Output:",
		"paths.output is a generated result directory",
		"does not keep",
		"roll back",
		"Exit codes:",
		"0  success, help, or version",
		"1  manifest, source, build, operation, or verify failure",
		"2  CLI argument error",
		"Examples:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("help output missing %q:\n%s", want, body)
		}
	}
}

// mainZipEntrySet 提取测试归档条目名称集合。
func mainZipEntrySet(t *testing.T, path string) map[string]bool {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	seen := map[string]bool{}
	for _, f := range zr.File {
		seen[f.Name] = true
	}
	return seen
}
