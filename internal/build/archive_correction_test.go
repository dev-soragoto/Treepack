package build

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepack/internal/logging"
)

func TestBuildUsesArchiveCorrectionsInLiteralStepsAndReport(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "src")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(sourceDir, "archive.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for name, body := range map[string]string{"CON?.txt": "config", "folder/app_1.bin": "payload"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "kit.toml")
	mustWriteBuildTest(t, manifestPath, `[pack]
name = "Portable"
[paths]
source = "src"
output = "out"
[[packages]]
name = "Archive Asset"
source = "file:archive.zip"
asset = "archive\\.zip"
install = "extract"
[[packages.steps]]
op = "cp"
from = "extract/archive.zip/CON?.txt"
to = "output/config.txt"
[[packages.steps]]
op = "cp_regex"
from = "extract/archive.zip/folder"
regex = "^app_.*\\.bin$"
to = "output/bin"
`)
	var logs bytes.Buffer
	rep, err := Build(Options{ConfigPath: manifestPath, Version: "test", Logger: logging.New(&logs)})
	if err != nil {
		t.Fatal(err)
	}
	assertFileContentBuild(t, filepath.Join(root, "out", "config.txt"), "config")
	assertFileContentBuild(t, filepath.Join(root, "out", "bin", "app_1.bin"), "payload")
	if len(rep.ArchiveCorrections) != 1 || len(rep.Failures) != 0 {
		t.Fatalf("corrections=%+v failures=%+v", rep.ArchiveCorrections, rep.Failures)
	}
	if !strings.Contains(logs.String(), "warning: archive entry renamed for portability:") {
		t.Fatalf("warning missing: %s", logs.String())
	}
	info, err := os.ReadFile(filepath.Join(root, "out", "BUILD_INFO.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Archive Path Corrections", "Package: Archive Asset", "Asset: archive.zip", "CON?.txt -> CON_.txt", "unsupported portable character"} {
		if !strings.Contains(string(info), want) {
			t.Fatalf("build info missing %q:\n%s", want, info)
		}
	}
}
