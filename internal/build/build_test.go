package build

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmokeBuild 验证 smoke 示例可以完成构建并生成预期输出。
func TestSmokeBuild(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	configPath := filepath.Join(repoRoot, "examples", "smoke", "kit.toml")
	outputDir := filepath.Join(repoRoot, "examples", "smoke-out")
	workBase := filepath.Join(repoRoot, "examples", ".treepack", "work")
	archivePath := filepath.Join(repoRoot, "examples", "smoke-2026.07.07.zip")
	_ = os.RemoveAll(outputDir)
	_ = os.RemoveAll(workBase)
	_ = os.Remove(archivePath)
	rep, err := Build(Options{ConfigPath: configPath, KeepWork: true, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.HasRequiredFailures() {
		t.Fatalf("unexpected required failures: %+v", rep.Failures)
	}
	if _, err := os.Stat(rep.Manifest.Paths.Work); err != nil {
		t.Fatalf("expected smoke work dir to be kept: %v", err)
	}
	for _, rel := range []string{
		"packages/001-Archive_Asset/extract/archive.zip/folder/app_payload_1.0.bin",
		"packages/001-Archive_Asset/output/bin/app-payload.bin",
		"packages/001-Archive_Asset/extract/archive.zip/old.delete",
		"packages/002-Single_File_Asset/output/bin/single.bin",
		"packages/003-Multi_Asset/output/multi/alpha.txt",
		"output/bin/app-payload.bin",
	} {
		if _, err := os.Stat(filepath.Join(rep.Manifest.Paths.Work, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected kept work path %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"bin/app-payload.bin",
		"bin/single.bin",
		"multi/alpha.txt",
		"multi/beta.txt",
		"config/resource.txt",
		"app/modules/module-a/state/enabled.flag",
		"empty/layout",
		"READY.txt",
		"BUILD_INFO.txt",
	} {
		if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected output %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "description.txt")); !os.IsNotExist(err) {
		t.Fatalf("description.txt should not be generated")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "old.delete")); !os.IsNotExist(err) {
		t.Fatalf("old.delete should be absent")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "overlay")); !os.IsNotExist(err) {
		t.Fatalf("overlay staging directory should be absent")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "extract")); !os.IsNotExist(err) {
		t.Fatalf("extract directory should not be published")
	}
	assertFileContentBuild(t, filepath.Join(outputDir, "bin", "app-payload.bin"), "payload fixture\n")
	assertFileContentBuild(t, filepath.Join(outputDir, "nested.txt"), "copied from overlay\n")
	assertFileContentBuild(t, filepath.Join(outputDir, "config", "resource.txt"), "resource copy ok\n")
	if len(rep.Packages) != 4 {
		t.Fatalf("expected 4 package records, got %d", len(rep.Packages))
	}
	if !rep.Packages[3].OK {
		t.Fatalf("optional package should install even when optional step fails: %+v", rep.Packages[3])
	}
	optionalFailure := false
	for _, result := range rep.Operations {
		if result.Label == "cp missing/optional.txt -> output/optional/missing.txt" && !result.OK && !result.Required {
			optionalFailure = true
			break
		}
	}
	if !optionalFailure {
		t.Fatalf("expected optional step failure to be recorded: %+v", rep.Operations)
	}
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	seen := map[string]bool{}
	for _, f := range zr.File {
		seen[f.Name] = true
		if filepath.Separator == '\\' && filepath.Clean(f.Name) == f.Name && filepath.ToSlash(filepath.Clean(f.Name)) != f.Name {
			t.Fatalf("zip path is not slash-normalized: %s", f.Name)
		}
	}
	for _, rel := range []string{"bin/", "bin/app-payload.bin", "bin/single.bin", "config/", "config/resource.txt", "empty/", "empty/layout/", "BUILD_INFO.txt"} {
		if !seen[rel] {
			t.Fatalf("zip missing expected file %s: %+v", rel, seen)
		}
	}
	if seen["old.delete"] || seen["overlay/nested.txt"] {
		t.Fatalf("zip contains files that should not be published: %+v", seen)
	}
}

func TestBuildChecksumRequiredFailureBlocksOutput(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "asset.bin"), "asset")
	mustWriteBuildTest(t, filepath.Join(outputDir, "old.txt"), "old")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Checksum"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "Asset"
source = "file:asset.bin"
asset = "asset\\.bin"
target = "asset.bin"
sha256 = "`+strings.Repeat("0", 64)+`"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	rep, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"})
	if err == nil || !strings.Contains(err.Error(), "required build step failed") {
		t.Fatalf("expected required checksum failure, got %v", err)
	}
	if rep == nil || len(rep.Failures) == 0 || !strings.Contains(rep.Failures[0], "sha256 mismatch") {
		t.Fatalf("expected checksum failure in report, got %+v", rep)
	}
	assertFileContentBuild(t, filepath.Join(outputDir, "old.txt"), "old")
}

func TestBuildChecksumOptionalFailureContinues(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "bad.bin"), "bad")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "good.bin"), "good")
	goodSum := sha256Hex("good")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Checksum Optional"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "Bad"
source = "file:bad.bin"
asset = "bad\\.bin"
required = false
sha256 = "`+strings.Repeat("0", 64)+`"
[[packages]]
name = "Good"
source = "file:good.bin"
asset = "good\\.bin"
target = "good.bin"
sha256 = "`+goodSum+`"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	rep, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failures) == 0 || !strings.Contains(rep.Failures[0], "sha256 mismatch") {
		t.Fatalf("expected optional checksum failure, got %+v", rep.Failures)
	}
	assertFileContentBuild(t, filepath.Join(outputDir, "good.bin"), "good")
}

func TestBuildRejectsSHA256ForDirectoryAsset(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "app", "run.txt"), "run")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Dir Checksum"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "App"
source = "file:app"
sha256 = "`+strings.Repeat("1", 64)+`"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	rep, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"})
	if err == nil || !strings.Contains(err.Error(), "required build step failed") {
		t.Fatalf("expected required package failure, got %v", err)
	}
	if rep == nil || len(rep.Failures) == 0 || !strings.Contains(rep.Failures[0], "sha256 is not supported for directory asset") {
		t.Fatalf("expected directory sha256 failure, got %+v", rep)
	}
}

func TestBuildArchiveFailurePreservesExistingOutput(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	archivePath := filepath.Join(root, "pack.zip")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "resources", "new.txt"), "new")
	mustWriteBuildTest(t, filepath.Join(outputDir, "old.txt"), "old")
	if err := os.MkdirAll(archivePath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteBuildTest(t, filepath.Join(archivePath+".tmp", "blocker"), "block")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Archive Fail"
[paths]
source = "SOURCE"
output = "OUTPUT"
[build]
archive = "ARCHIVE"
[resources]
copy = "resources"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	rewriteMainTestFileForBuild(t, filepath.Join(root, "kit.toml"), "ARCHIVE", filepath.ToSlash(archivePath))
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err == nil {
		t.Fatal("expected archive creation failure")
	}
	assertFileContentBuild(t, filepath.Join(outputDir, "old.txt"), "old")
	if _, err := os.Stat(filepath.Join(outputDir, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new output should not be published after archive failure")
	}
}

// TestBuildRejectsPackageTargetTraversal 验证 package target 不能逃出输出目录。
func TestBuildRejectsPackageTargetTraversal(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "asset.bin"), "asset")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Bad"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "Asset"
source = "file:asset.bin"
asset = "asset\\.bin"
target = "../outside.bin"
`)
	body, err := os.ReadFile(filepath.Join(root, "kit.toml"))
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "SOURCE", filepath.ToSlash(sourceDir)), "OUTPUT", filepath.ToSlash(filepath.Join(root, "out"))))
	if err := os.WriteFile(filepath.Join(root, "kit.toml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err == nil {
		t.Fatal("expected traversal target to fail")
	}
	if _, err := os.Stat(filepath.Join(root, "outside.bin")); !os.IsNotExist(err) {
		t.Fatal("outside file should not be created")
	}
}

// TestBuildRejectsExtractStagingNameCollision 验证对应场景的行为。
func TestBuildRejectsExtractStagingNameCollision(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	makeTestZip(t, filepath.Join(sourceDir, "foo bar.zip"), map[string]string{"a.txt": "a"})
	makeTestZip(t, filepath.Join(sourceDir, "foo@bar.zip"), map[string]string{"b.txt": "b"})
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Collision"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "Archives"
source = "file:."
[[packages.assets]]
asset = "foo bar\\.zip"
install = "extract"
[[packages.assets]]
asset = "foo@bar\\.zip"
install = "extract"
`)
	body, err := os.ReadFile(filepath.Join(root, "kit.toml"))
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "SOURCE", filepath.ToSlash(sourceDir)), "OUTPUT", filepath.ToSlash(filepath.Join(root, "out"))))
	if err := os.WriteFile(filepath.Join(root, "kit.toml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"})
	if err == nil || !strings.Contains(err.Error(), "required build step failed") {
		t.Fatalf("expected required package failure, got %v", err)
	}
}

// TestBuildSkipsArchive 验证未声明 archive 时只生成目录树而不创建 zip。
func TestBuildSkipsArchive(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Directory Only"
[paths]
source = "SOURCE"
output = "OUTPUT"
`)
	body, err := os.ReadFile(filepath.Join(root, "kit.toml"))
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "SOURCE", filepath.ToSlash(sourceDir)), "OUTPUT", filepath.ToSlash(filepath.Join(root, "out"))))
	if err := os.WriteFile(filepath.Join(root, "kit.toml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Manifest.Paths.Output != filepath.Join(root, "out") {
		t.Fatalf("unexpected output: %s", rep.Manifest.Paths.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "BUILD_INFO.txt")); err != nil {
		t.Fatalf("expected build info: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "directory-only.zip")); !os.IsNotExist(err) {
		t.Fatal("archive should not be created")
	}
}

// TestBuildArchiveFiltersMetadataWithoutChangingOutput 验证默认 ZIP 过滤系统元数据，但最终 output 目录保持原样。
func TestBuildArchiveFiltersMetadataWithoutChangingOutput(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "resources", "keep.txt"), "keep")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "resources", ".DS_Store"), "ds")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "resources", "__MACOSX", "metadata.txt"), "metadata")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Archive Filter"
[paths]
source = "SOURCE"
output = "OUTPUT"
[build]
archive = "filtered.zip"
[resources]
copy = "resources"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".DS_Store", "__MACOSX/metadata.txt"} {
		if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected output metadata %s: %v", rel, err)
		}
	}
	seen := buildZipEntrySet(t, filepath.Join(root, "filtered.zip"))
	if !seen["keep.txt"] {
		t.Fatalf("archive missing keep.txt: %+v", seen)
	}
	for _, name := range []string{".DS_Store", "__MACOSX/", "__MACOSX/metadata.txt"} {
		if seen[name] {
			t.Fatalf("archive should skip metadata %s: %+v", name, seen)
		}
	}
}

// TestBuildRawArchiveIncludesMetadata 验证 RawArchive 会按 output 目录原样归档系统元数据。
func TestBuildRawArchiveIncludesMetadata(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "resources", ".DS_Store"), "ds")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "resources", "__MACOSX", "metadata.txt"), "metadata")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Raw Archive"
[paths]
source = "SOURCE"
output = "OUTPUT"
[build]
archive = "raw.zip"
[resources]
copy = "resources"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), RawArchive: true, Version: "test"}); err != nil {
		t.Fatal(err)
	}
	seen := buildZipEntrySet(t, filepath.Join(root, "raw.zip"))
	for _, name := range []string{".DS_Store", "__MACOSX/", "__MACOSX/metadata.txt"} {
		if !seen[name] {
			t.Fatalf("raw archive missing metadata %s: %+v", name, seen)
		}
	}
}

// TestBuildRequiresSourceAndOutput 验证 source/output 必须由 TOML 或 CLI 提供。
func TestBuildRequiresSourceAndOutput(t *testing.T) {
	root := t.TempDir()
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Missing Paths"
`)
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err == nil {
		t.Fatal("expected missing paths to fail")
	}
}

// TestBuildKeepWorkControlsCleanup 验证默认清理 work，keep_work 会保留 run dir。
func TestBuildKeepWorkControlsCleanup(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Keep Work"
`)
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Source: sourceDir, Output: outputDir, WorkDir: workDir, Version: "test"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected work dir to be empty after cleanup, got %d entries", len(entries))
	}
	rep, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Source: sourceDir, Output: outputDir, WorkDir: workDir, KeepWork: true, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rep.Manifest.Paths.Work); err != nil {
		t.Fatalf("expected kept work dir: %v", err)
	}
}

// TestBuildMergesPackagesInManifestOrder 验证后面的 package 覆盖前面的 package。
func TestBuildMergesPackagesInManifestOrder(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "a.txt"), "first")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "b.txt"), "second")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `
[pack]
name = "Merge"
[[packages]]
name = "First"
source = "file:a.txt"
asset = "a\\.txt"
target = "same.txt"
[[packages]]
name = "Second"
source = "file:b.txt"
asset = "b\\.txt"
target = "same.txt"
`)
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Source: sourceDir, Output: outputDir, Version: "test"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "same.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("expected second package to win, got %q", data)
	}
}

// TestBuildRejectsSymlinkSource 验证 paths.source 本身是 symlink 时会被真实目录边界检查拒绝。
func TestBuildRejectsSymlinkSource(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("symlink creation on Windows may require privileges")
	}
	root := t.TempDir()
	realSource := filepath.Join(root, "real-src")
	if err := os.MkdirAll(realSource, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceLink := filepath.Join(root, "source-link")
	if err := os.Symlink(realSource, sourceLink); err != nil {
		t.Fatal(err)
	}
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Symlink Source"
[paths]
source = "SOURCE"
output = "OUTPUT"
`)
	body, err := os.ReadFile(filepath.Join(root, "kit.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(body), "SOURCE", filepath.ToSlash(sourceLink)), "OUTPUT", filepath.ToSlash(filepath.Join(root, "out")))
	if err := os.WriteFile(filepath.Join(root, "kit.toml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err == nil {
		t.Fatal("expected symlink paths.source to fail")
	}
}

// TestManifestRelativePathsResolveFromManifestDirectory 验证对应场景的行为。
func TestManifestRelativePathsResolveFromManifestDirectory(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	sourceDir := filepath.Join(project, "src")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "asset.txt"), "asset")
	mustWriteBuildTest(t, filepath.Join(project, "kit.toml"), `[pack]
name = "Relative"
[paths]
source = "src"
output = "out"
work = "work"
[build]
archive = "archives/{pack.name}.zip"
[[packages]]
name = "Asset"
source = "file:asset.txt"
asset = "asset\\.txt"
target = "asset.txt"
`)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	rep, err := Build(Options{ConfigPath: filepath.Join(project, "kit.toml"), Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Manifest.Paths.Source != sourceDir {
		t.Fatalf("source = %s, want %s", rep.Manifest.Paths.Source, sourceDir)
	}
	if _, err := os.Stat(filepath.Join(project, "out", "asset.txt")); err != nil {
		t.Fatalf("expected manifest-relative output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "archives", "Relative.zip")); err != nil {
		t.Fatalf("expected manifest-relative archive: %v", err)
	}
}

// TestCLIOverridePathsResolveFromCurrentDirectory 验证对应场景的行为。
func TestCLIOverridePathsResolveFromCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	manifestDir := filepath.Join(root, "manifest")
	cwd := filepath.Join(root, "cwd")
	mustWriteBuildTest(t, filepath.Join(cwd, "src", "asset.txt"), "asset")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteBuildTest(t, filepath.Join(manifestDir, "kit.toml"), `[pack]
name = "CLI Relative"
[[packages]]
name = "Asset"
source = "file:asset.txt"
asset = "asset\\.txt"
target = "asset.txt"
`)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	rep, err := Build(Options{
		ConfigPath: filepath.Join(manifestDir, "kit.toml"),
		Source:     "src",
		Output:     "out",
		WorkDir:    "work",
		KeepWork:   true,
		Version:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Manifest.Paths.Output != filepath.Join(cwd, "out") {
		t.Fatalf("output = %s, want %s", rep.Manifest.Paths.Output, filepath.Join(cwd, "out"))
	}
	if _, err := os.Stat(filepath.Join(cwd, "out", "asset.txt")); err != nil {
		t.Fatalf("expected cwd-relative output: %v", err)
	}
}

// TestBuildRejectsUnsafeArchivePaths 验证对应场景的行为。
func TestBuildRejectsUnsafeArchivePaths(t *testing.T) {
	for _, tc := range []struct {
		name    string
		archive string
		setup   func(root string)
	}{
		{name: "source", archive: "src/archive.zip"},
		{name: "output", archive: "out/archive.zip"},
		{name: "work", archive: "work/archive.zip"},
		{name: "manifest", archive: "kit.toml"},
		{name: "existing-dir", archive: "existing", setup: func(root string) {
			if err := os.MkdirAll(filepath.Join(root, "existing"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			sourceDir := filepath.Join(root, "src")
			if err := os.MkdirAll(sourceDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.setup != nil {
				tc.setup(root)
			}
			mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Unsafe Archive"
[paths]
source = "SRC"
output = "OUT"
work = "WORK"
[build]
archive = "ARCHIVE"
`)
			body, err := os.ReadFile(filepath.Join(root, "kit.toml"))
			if err != nil {
				t.Fatal(err)
			}
			replacements := map[string]string{
				"SRC":     filepath.ToSlash(sourceDir),
				"OUT":     filepath.ToSlash(filepath.Join(root, "out")),
				"WORK":    filepath.ToSlash(filepath.Join(root, "work")),
				"ARCHIVE": filepath.ToSlash(filepath.Join(root, filepath.FromSlash(tc.archive))),
			}
			text := string(body)
			for old, value := range replacements {
				text = strings.ReplaceAll(text, old, value)
			}
			if err := os.WriteFile(filepath.Join(root, "kit.toml"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err == nil {
				t.Fatal("expected unsafe archive path to fail")
			}
		})
	}
}

// TestBuildArchiveTemplateValuesUseStrictSanitizer 验证对应场景的行为。
func TestBuildArchiveTemplateValuesUseStrictSanitizer(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     string
	}{
		{name: "name", template: "{pack.name}.zip", want: ".._Bad_Name_Pack_.zip"},
		{name: "safe-name", template: "{pack.safe_name}.zip", want: ".._Bad_Name_Pack_.zip"},
		{name: "version", template: "{pack.version}.zip", want: ".._1_2_3_4_5_6_7_8_9.zip"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			sourceDir := filepath.Join(root, "src")
			if err := os.MkdirAll(sourceDir, 0o755); err != nil {
				t.Fatal(err)
			}
			mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "../Bad Name:Pack?"
version = '../1\2 3<4>5:6|7?8*9'
[paths]
source = "SOURCE"
output = "OUTPUT"
[build]
archive = "ARCHIVE/TEMPLATE"
`)
			body, err := os.ReadFile(filepath.Join(root, "kit.toml"))
			if err != nil {
				t.Fatal(err)
			}
			text := string(body)
			replacements := map[string]string{
				"SOURCE":   filepath.ToSlash(sourceDir),
				"OUTPUT":   filepath.ToSlash(filepath.Join(root, "out")),
				"ARCHIVE":  filepath.ToSlash(filepath.Join(root, "archives")),
				"TEMPLATE": tc.template,
			}
			for old, value := range replacements {
				text = strings.ReplaceAll(text, old, value)
			}
			if err := os.WriteFile(filepath.Join(root, "kit.toml"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(root, "archives", tc.want)); err != nil {
				t.Fatalf("expected sanitized archive name %s: %v", tc.want, err)
			}
		})
	}
}

// TestBuildArchiveTemplateRejectsUnknownPlaceholder 验证对应场景的行为。
func TestBuildArchiveTemplateRejectsUnknownPlaceholder(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Bad Placeholder"
[paths]
source = "SOURCE"
output = "OUTPUT"
[build]
archive = "archive-{pack.raw_name}.zip"
`)
	body, err := os.ReadFile(filepath.Join(root, "kit.toml"))
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "SOURCE", filepath.ToSlash(sourceDir)), "OUTPUT", filepath.ToSlash(filepath.Join(root, "out"))))
	if err := os.WriteFile(filepath.Join(root, "kit.toml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err == nil || !strings.Contains(err.Error(), "unknown template placeholder") {
		t.Fatalf("expected unknown placeholder error, got %v", err)
	}
}

// TestBuildInstallsLocalDirectorySourceDefaultTarget 验证本地目录 source 默认保留目录名。
func TestBuildInstallsLocalDirectorySourceDefaultTarget(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "app", "bin", "run.txt"), "run")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Dir"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "App"
source = "file:app"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err != nil {
		t.Fatal(err)
	}
	assertFileContentBuild(t, filepath.Join(outputDir, "app", "bin", "run.txt"), "run")
}

// TestBuildInstallsLocalDirectorySourceCustomTarget 验证本地目录 source 可安装到指定 target。
func TestBuildInstallsLocalDirectorySourceCustomTarget(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "app", "bin", "run.txt"), "run")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Dir"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "App"
source = "file:app"
target = "program"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err != nil {
		t.Fatal(err)
	}
	assertFileContentBuild(t, filepath.Join(outputDir, "program", "bin", "run.txt"), "run")
}

// TestBuildInstallsLocalDirectorySourceMergeTarget 验证 target "." 会合并目录内容到输出根。
func TestBuildInstallsLocalDirectorySourceMergeTarget(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "app", "bin", "run.txt"), "run")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Dir"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "App"
source = "file:app"
target = "."
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err != nil {
		t.Fatal(err)
	}
	assertFileContentBuild(t, filepath.Join(outputDir, "bin", "run.txt"), "run")
	if _, err := os.Stat(filepath.Join(outputDir, "app")); !os.IsNotExist(err) {
		t.Fatalf("app directory should not be created for merge target")
	}
}

// TestBuildRejectsExtractForDirectoryAsset 验证目录资产不能使用 extract 安装。
func TestBuildRejectsExtractForDirectoryAsset(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "app", "bin", "run.txt"), "run")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Dir"
[paths]
source = "SOURCE"
output = "OUTPUT"
[[packages]]
name = "App"
source = "file:app"
install = "extract"
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	rep, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"})
	if err == nil || !strings.Contains(err.Error(), "required build step failed") {
		t.Fatalf("expected required package failure, got %v", err)
	}
	if rep == nil || len(rep.Failures) == 0 || !strings.Contains(rep.Failures[0], "extract only supports zip assets") {
		t.Fatalf("expected extract failure in report, got %+v", rep)
	}
}

// rewriteBuildManifestPaths replaces test placeholders with platform-independent absolute paths.
func rewriteBuildManifestPaths(t *testing.T, path, sourceDir, outputDir string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "SOURCE", filepath.ToSlash(sourceDir)), "OUTPUT", filepath.ToSlash(outputDir)))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func rewriteMainTestFileForBuild(t *testing.T, path, old, newValue string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), old, newValue))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// mustWriteBuildTest 写入测试文件，失败时终止当前测试。
func mustWriteBuildTest(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertFileContentBuild 断言测试期望，失败时终止当前测试。
func assertFileContentBuild(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}

// makeTestZip 创建测试所需的 zip 归档内容。
func makeTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
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

// buildZipEntrySet 提取测试归档条目名称集合。
func buildZipEntrySet(t *testing.T, path string) map[string]bool {
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
