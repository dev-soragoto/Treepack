package archive

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractZipCorrectsPortableNames(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "portable.zip")
	writeZipEntries(t, zipPath, []zipEntry{
		{name: "folder./bad?.txt", body: "bad"},
		{name: "CON.txt", body: "device"},
		{name: "正常 名称.txt", body: "unicode"},
	})
	result, err := ExtractZip(zipPath, filepath.Join(root, "out"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"folder_/bad_.txt", "_CON.txt", "正常 名称.txt"} {
		if _, err := os.Stat(filepath.Join(root, "out", filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	if len(result.Corrections) != 2 {
		t.Fatalf("corrections = %+v", result.Corrections)
	}
	if got := strings.Join(result.Corrections[0].Reasons, "; "); got != "trailing dot; unsupported portable character" {
		t.Fatalf("reasons = %q", got)
	}
}

func TestCorrectZipPathPortableCorrections(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		reasons []string
	}{
		{name: "unchanged", input: "folder/normal file.txt", want: "folder/normal file.txt"},
		{name: "device name", input: "CON.txt", want: "_CON.txt", reasons: []string{"Windows reserved device name"}},
		{name: "portable characters", input: `bad<name?.txt`, want: "bad_name_.txt", reasons: []string{"unsupported portable character"}},
		{name: "control character", input: "bad\x01name.txt", want: "bad_name.txt", reasons: []string{"control character"}},
		{name: "trailing dot and space", input: "name. ", want: "name__", reasons: []string{"trailing dot", "trailing space"}},
		{name: "multiple components", input: "dir./AUX /ok?.txt", want: "dir_/_AUX_/ok_.txt", reasons: []string{"trailing dot", "trailing space", "Windows reserved device name", "unsupported portable character"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, corrections := CorrectZipPath(tc.input)
			if got != tc.want {
				t.Fatalf("CorrectZipPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
			if len(tc.reasons) == 0 {
				if corrections != nil {
					t.Fatalf("unchanged path corrections = %#v, want nil", corrections)
				}
				return
			}
			if len(corrections) != 1 {
				t.Fatalf("corrections = %#v, want one", corrections)
			}
			correction := corrections[0]
			if correction.Original != tc.input || correction.Extracted != tc.want {
				t.Fatalf("correction metadata = %#v", correction)
			}
			gotReasons := make(map[string]bool, len(correction.Reasons))
			for _, reason := range correction.Reasons {
				gotReasons[reason] = true
			}
			if len(gotReasons) != len(tc.reasons) {
				t.Fatalf("reasons = %#v, want %#v", correction.Reasons, tc.reasons)
			}
			for _, reason := range tc.reasons {
				if !gotReasons[reason] {
					t.Fatalf("reasons = %#v, missing %q", correction.Reasons, reason)
				}
			}
		})
	}
}

func TestExtractZipRejectsOverlongComponentWithoutChangingTarget(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "long.zip")
	writeZipEntries(t, zipPath, []zipEntry{{name: strings.Repeat("a", 256), body: "bad"}})
	out := filepath.Join(root, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "sentinel.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractZip(zipPath, out); err == nil || !strings.Contains(err.Error(), "component exceeds 255 bytes") {
		t.Fatalf("expected portable component length error, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "sentinel.txt"))
	if err != nil || string(data) != "old" {
		t.Fatalf("preflight changed existing target: %q, %v", data, err)
	}
}

func TestExtractZipRejectsPortableConflictsWithoutChangingTarget(t *testing.T) {
	tests := [][]zipEntry{
		{{name: "a?.txt"}, {name: "a*.txt"}},
		{{name: "Readme"}, {name: "README"}},
		{{name: "caf\u00e9"}, {name: "cafe\u0301"}},
		{{name: "a"}, {name: "a/b.txt"}},
		{{name: "a", body: "file"}, {name: "a/"}},
	}
	for i, entries := range tests {
		root := t.TempDir()
		zipPath := filepath.Join(root, "bad.zip")
		writeZipEntries(t, zipPath, entries)
		out := filepath.Join(root, "out")
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "sentinel"), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ExtractZip(zipPath, out); err == nil {
			t.Fatalf("case %d should fail", i)
		}
		data, err := os.ReadFile(filepath.Join(out, "sentinel"))
		if err != nil || string(data) != "old" {
			t.Fatalf("case %d changed target: %q, %v", i, data, err)
		}
	}
}

func TestExtractZipRejectsAbsoluteAndDrivePaths(t *testing.T) {
	for _, name := range []string{"/absolute", `\\server\share`, "C:/drive", "C:drive", "a/../b", "a/./b"} {
		t.Run(name, func(t *testing.T) {
			zipPath := filepath.Join(t.TempDir(), "bad.zip")
			writeZipEntries(t, zipPath, []zipEntry{{name: name}})
			if _, err := ExtractZip(zipPath, filepath.Join(t.TempDir(), "out")); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestPlanExtractionAllowsExplicitDirectories(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "dirs.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	if _, err := zw.Create("a/"); err != nil {
		t.Fatal(err)
	}
	w, err := zw.Create("a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("body"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractZip(zipPath, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
}

func TestExtractZipPreflightFailureDoesNotCreateTarget(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "bad.zip")
	writeZipEntries(t, zipPath, []zipEntry{{name: "../escape"}})
	out := filepath.Join(root, "extract", "asset")
	if _, err := ExtractZip(zipPath, out); err == nil {
		t.Fatal("expected rejection")
	}
	if _, err := os.Stat(filepath.Join(root, "extract")); !os.IsNotExist(err) {
		t.Fatalf("preflight created target parent: %v", err)
	}
}

func TestExtractZipContentFailureCleansTemporaryTarget(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "corrupt.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	header := &zip.FileHeader{Name: "file.txt", Method: zip.Store}
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("treepack-crc-body")
	_, _ = w.Write(body)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	index := bytes.Index(data, body)
	if index < 0 {
		t.Fatal("stored body not found")
	}
	data[index] ^= 0xff
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "extract")
	if _, err := ExtractZip(zipPath, target); err == nil {
		t.Fatal("expected CRC failure")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("corrupt archive left target: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".treepack-extract-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary extraction leftovers: %v, %v", matches, err)
	}
}
