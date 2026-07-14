package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplainRejectsUnsafeStaticConfigurationWithoutCreatingBuildPaths(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(root, source, output, work string) (configPath, extra string)
		want       []string
		outputMade bool
	}{
		{
			name: "source output overlap",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(root, "kit.toml"), ""
			},
			want: []string{"paths.source and paths.output cannot overlap"},
		},
		{
			name: "work overlap",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(root, "kit.toml"), "WORK_OVERLAP"
			},
			want: []string{"paths.work cannot overlap"},
		},
		{
			name: "manifest inside output",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(output, "kit.toml"), ""
			},
			want:       []string{"config file cannot be inside output"},
			outputMade: true,
		},
		{
			name: "package target traversal",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(root, "kit.toml"), `[[packages]]
name = "Bad target"
source = "file:missing.bin"
target = "../outside.bin"
`
			},
			want: []string{"packages[1].target", "path escapes base directory"},
		},
		{
			name: "step traversal",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(root, "kit.toml"), `[[packages]]
name = "Bad step"
source = "file:missing.bin"
[[packages.steps]]
op = "cp"
from = "../outside.bin"
to = "output/file.bin"
`
			},
			want: []string{"packages[1].steps[1].from", "path escapes base directory"},
		},
		{
			name: "layout traversal",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(root, "kit.toml"), "[layout]\ndirs = [\"../layout-outside\"]\n"
			},
			want: []string{"path escapes base directory", "../layout-outside"},
		},
		{
			name: "verify traversal",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(root, "kit.toml"), "[verify]\nfiles = [\"../verify-outside\"]\n"
			},
			want: []string{"path escapes base directory", "../verify-outside"},
		},
		{
			name: "build info traversal",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(root, "kit.toml"), "[build]\nbuild_info = \"../BUILD_INFO.txt\"\n"
			},
			want: []string{"build.build_info", "path escapes base directory"},
		},
		{
			name: "archive overlaps output",
			configure: func(root, source, output, work string) (string, string) {
				return filepath.Join(root, "kit.toml"), "[build]\narchive = \"" + filepath.ToSlash(filepath.Join(output, "pack.zip")) + "\"\n"
			},
			want: []string{"build.archive cannot overlap a build path"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "missing-source")
			output := filepath.Join(root, "missing-output")
			work := filepath.Join(root, "missing-work")
			if tc.name == "source output overlap" {
				output = filepath.Join(source, "nested-output")
			}
			configPath, extra := tc.configure(root, source, output, work)
			if extra == "WORK_OVERLAP" {
				work = filepath.Join(output, "nested-work")
				extra = ""
			}
			manifest := `[pack]
name = "Explain Safety"
[paths]
source = "` + filepath.ToSlash(source) + `"
output = "` + filepath.ToSlash(output) + `"
work = "` + filepath.ToSlash(work) + `"
` + extra
			mustWriteBuildTest(t, configPath, manifest)

			if _, err := Explain(Options{ConfigPath: configPath}); err == nil {
				t.Fatal("expected Explain preflight to fail")
			} else {
				for _, want := range tc.want {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("error %q does not contain %q", err, want)
					}
				}
			}
			for _, path := range []string{source, work} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("Explain created %s: %v", path, err)
				}
			}
			if !tc.outputMade {
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatalf("Explain created output %s: %v", output, err)
				}
			}
		})
	}
}

func TestExplainDoesNotReadMissingSource(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "kit.toml")
	source := filepath.Join(root, "source-does-not-exist")
	output := filepath.Join(root, "output-does-not-exist")
	mustWriteBuildTest(t, configPath, `[pack]
name = "Static Explain"
[paths]
source = "`+filepath.ToSlash(source)+`"
output = "`+filepath.ToSlash(output)+`"
[[packages]]
name = "Missing local asset"
source = "file:missing.bin"
target = "asset.bin"
`)
	text, err := Explain(Options{ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "file:missing.bin") {
		t.Fatalf("explain output did not describe missing source:\n%s", text)
	}
	for _, path := range []string{source, output} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Explain created %s: %v", path, err)
		}
	}
}
