package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainIsStaticAndShowsOrderedOperations(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "kit.toml")
	writeMainTestFile(t, manifestPath, `[pack]
name = "Explain Kit"
version = "1.0"
[paths]
source = "missing-source"
output = "out"
work = "work"
[build]
archive = "pack-{pack.version}.zip"
[layout]
dirs = ["empty"]
[[packages]]
name = "Archive"
source = "url:https://example.invalid/archive.zip"
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
[[packages.steps]]
op = "rm"
path = "output/old.delete"
[[packages.steps]]
op = "touch"
path = "output/READY.txt"
[verify]
files = ["config.txt"]
`)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", manifestPath, "--explain"}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	body := stdout.String()
	for _, want := range []string{
		"Resolved paths", "[1.1] Extract asset: Archive [required]",
		"<archive contents not inspected>", "[1.2] cp [required]",
		"config.txt -> package extract/{asset:archive\\.zip}/CON_.txt",
		"portable correction:\n└── CON?.txt -> CON_.txt", "[1.3] cp_regex [required]",
		"<matches:^app_.*\\.bin$>", "[1.4] rm [required]", "old.delete [remove]",
		"[1.5] touch [required]", "READY.txt -> generated: touch",
		"Merge package: Archive", "layout", "verify", "build_info", "archive", "publish",
		"archive contents are not inspected; cp_regex matches extracted physical names",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("explain missing %q:\n%s", want, body)
		}
	}
	for _, path := range []string{filepath.Join(root, "missing-source"), filepath.Join(root, "out"), filepath.Join(root, "work"), filepath.Join(root, "pack-1.0.zip")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("explain created or accessed output path %s: %v", path, err)
		}
	}
}

func TestExplainHonorsPathOverrides(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "kit.toml")
	writeMainTestFile(t, manifestPath, "[pack]\nname='Overrides'\n[paths]\nsource='source'\noutput='output'\n")
	var stdout, stderr bytes.Buffer
	source := filepath.Join(root, "cli-source")
	output := filepath.Join(root, "cli-output")
	if code := run([]string{"--config", manifestPath, "--source", source, "--output", output, "--explain"}, &stdout, &stderr); code != 0 {
		t.Fatalf("explain returned %d: %s", code, stderr.String())
	}
	for _, want := range []string{filepath.ToSlash(source), filepath.ToSlash(output), "--source " + source, "--output " + output} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("missing override %q:\n%s", want, stdout.String())
		}
	}
}
