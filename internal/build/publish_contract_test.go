package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildReplacesPublishedTreeAndArchive(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	outputDir := filepath.Join(root, "out")
	archivePath := filepath.Join(root, "pack.zip")

	mustWriteBuildTest(t, filepath.Join(sourceDir, "payload", "fresh.txt"), "fresh")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "payload", "file-wins"), "new file")
	mustWriteBuildTest(t, filepath.Join(sourceDir, "payload", "dir-wins", "nested.txt"), "new nested")
	mustWriteBuildTest(t, filepath.Join(outputDir, "stale.txt"), "stale")
	mustWriteBuildTest(t, filepath.Join(outputDir, "stale-dir", "old.txt"), "old")
	mustWriteBuildTest(t, filepath.Join(outputDir, "file-wins", "old.txt"), "old directory")
	mustWriteBuildTest(t, filepath.Join(outputDir, "dir-wins"), "old file")
	mustWriteBuildTest(t, archivePath, "old archive")
	mustWriteBuildTest(t, filepath.Join(root, "kit.toml"), `[pack]
name = "Publish Replacement"
[paths]
source = "SOURCE"
output = "OUTPUT"
[build]
archive = "ARCHIVE"
[[packages]]
name = "Payload"
source = "file:payload"
target = "."
`)
	rewriteBuildManifestPaths(t, filepath.Join(root, "kit.toml"), sourceDir, outputDir)
	rewriteMainTestFileForBuild(t, filepath.Join(root, "kit.toml"), "ARCHIVE", filepath.ToSlash(archivePath))

	if _, err := Build(Options{ConfigPath: filepath.Join(root, "kit.toml"), Version: "test"}); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{"stale.txt", "stale-dir"} {
		if _, err := os.Stat(filepath.Join(outputDir, stale)); !os.IsNotExist(err) {
			t.Fatalf("stale output %s remains: %v", stale, err)
		}
	}
	assertFileContentBuild(t, filepath.Join(outputDir, "fresh.txt"), "fresh")
	assertFileContentBuild(t, filepath.Join(outputDir, "file-wins"), "new file")
	assertFileContentBuild(t, filepath.Join(outputDir, "dir-wins", "nested.txt"), "new nested")
	if info, err := os.Stat(filepath.Join(outputDir, "BUILD_INFO.txt")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("BUILD_INFO.txt was not published as a file: %v", err)
	}

	entries := buildZipEntrySet(t, archivePath)
	for _, want := range []string{"fresh.txt", "file-wins", "dir-wins/", "dir-wins/nested.txt", "BUILD_INFO.txt"} {
		if !entries[want] {
			t.Fatalf("replacement archive missing %s: %#v", want, entries)
		}
	}
	for _, stale := range []string{"stale.txt", "stale-dir/", "stale-dir/old.txt", "file-wins/old.txt"} {
		if entries[stale] {
			t.Fatalf("replacement archive contains stale entry %s: %#v", stale, entries)
		}
	}
}
