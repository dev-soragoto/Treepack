package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadDefaultsAndValidation 验证 manifest 加载会补默认值并通过基础校验。
func TestLoadDefaultsAndValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kit.toml")
	if err := os.WriteFile(path, []byte(`
[pack]
name = "Demo"
[[packages]]
name = "Local"
source = "file:asset.bin"
asset = "asset\\.bin"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Build.Archive != "" || m.Build.BuildInfo != "BUILD_INFO.txt" {
		t.Fatalf("defaults not applied: %+v", m.Build)
	}
	if m.Packages[0].Group != "default" || !m.Packages[0].IsRequired() {
		t.Fatalf("package defaults not applied: %+v", m.Packages[0])
	}
}

func TestLoadAcceptsSHA256Pins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kit.toml")
	sum := strings.Repeat("a", 64)
	if err := os.WriteFile(path, []byte(`
[pack]
name = "Demo"
[[packages]]
name = "Single"
source = "file:asset.bin"
asset = "asset\\.bin"
sha256 = "`+sum+`"
[[packages]]
name = "Multi"
source = "file:."
[[packages.assets]]
asset = "a\\.bin"
sha256 = "`+sum+`"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Packages[0].SHA256 != sum || m.Packages[1].Assets[0].SHA256 != sum {
		t.Fatalf("sha256 pins not loaded: %+v", m.Packages)
	}
}

// TestLoadRejectsUnknownKeys 验证未知配置键会被拒绝。
func TestLoadRejectsUnknownKeys(t *testing.T) {
	tests := []string{
		`[pack]
name = "Demo"
[build]
unknown = "value"
`,
		`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
[[packages.assets]]
asset = "x"
required = true
`,
		`[pack]
name = "Demo"
[resources]
copy = "resources"
`,
	}
	for _, body := range tests {
		dir := t.TempDir()
		path := filepath.Join(dir, "kit.toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("expected unknown key error for:\n%s", body)
		}
	}
}

func TestLoadRejectsInvalidSHA256(t *testing.T) {
	tests := []string{
		`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
asset = "x"
sha256 = "not-hex"
`,
		`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
[[packages.assets]]
asset = "x"
sha256 = "` + strings.Repeat("g", 64) + `"
`,
	}
	for _, body := range tests {
		dir := t.TempDir()
		path := filepath.Join(dir, "kit.toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("expected invalid sha256 error for:\n%s", body)
		}
	}
}

func TestLoadManifestNotFoundSentinel(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if !errors.Is(err, ErrManifestNotFound) {
		t.Fatalf("expected ErrManifestNotFound, got %v", err)
	}
}

// TestLoadRejectsAssetAndAssetsConflict 验证对应场景的行为。
func TestLoadRejectsAssetAndAssetsConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kit.toml")
	if err := os.WriteFile(path, []byte(`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
asset = "x"
[[packages.assets]]
asset = "y"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected asset/assets conflict")
	}
}

// TestLoadRejectsInvalidRegexAtLoadTime 验证对应场景的行为。
func TestLoadRejectsInvalidRegexAtLoadTime(t *testing.T) {
	for _, body := range []string{
		`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
asset = "["
`,
		`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
[[packages.steps]]
op = "cp_regex"
from = "."
regex = "["
to = "out"
`,
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "kit.toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("expected regex validation error for:\n%s", body)
		}
	}
}

// TestLoadRejectsInvalidOpSourceAndInstall 验证非法 source、install 和操作类型会被拒绝。
func TestLoadRejectsInvalidOpSourceAndInstall(t *testing.T) {
	tests := []string{
		`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "ftp:x"
`,
		`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
install = "copy"
`,
		`[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
[[packages.steps]]
op = "rename"
path = "x"
`,
	}
	for _, body := range tests {
		dir := t.TempDir()
		path := filepath.Join(dir, "kit.toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("expected validation error for:\n%s", body)
		}
	}
}

// TestLoadValidatesCpRegexRequiredFields 验证对应场景的行为。
func TestLoadValidatesCpRegexRequiredFields(t *testing.T) {
	tests := []string{
		`regex = ".*"
to = "out"`,
		`from = "in"
to = "out"`,
		`from = "in"
regex = ".*"`,
	}
	for _, stepBody := range tests {
		dir := t.TempDir()
		path := filepath.Join(dir, "kit.toml")
		body := `[pack]
name = "Demo"
[[packages]]
name = "Bad"
source = "file:x"
[[packages.steps]]
op = "cp_regex"
` + stepBody + "\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("expected cp_regex validation error for:\n%s", body)
		}
	}
}

func TestLoadValidatesURLAssetCardinalityAndExtractTarget(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "url multiple assets",
			body: `[[packages]]
name = "URL"
source = "url:https://example.com/a.zip"
[[packages.assets]]
asset = "a"
[[packages.assets]]
asset = "b"`,
			want: `package(name: "URL").assets`,
		},
		{
			name: "package extract target",
			body: `[[packages]]
name = "Extract"
source = "file:a.zip"
asset = "a"
install = "extract"
target = "ignored"`,
			want: `package(name: "Extract").target`,
		},
		{
			name: "asset extract target",
			body: `[[packages]]
name = "Extract"
source = "file:."
[[packages.assets]]
asset = "a"
install = "extract"
target = "ignored"`,
			want: `package(name: "Extract").assets[1].target`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kit.toml")
			body := "[pack]\nname = \"Demo\"\n" + tc.body + "\n"
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}

	path := filepath.Join(t.TempDir(), "kit.toml")
	body := `[pack]
name = "Demo"
[[packages]]
name = "URL"
source = "url:https://example.com/a.zip"
[[packages.assets]]
asset = "a"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("single URL assets entry should be valid: %v", err)
	}
}
