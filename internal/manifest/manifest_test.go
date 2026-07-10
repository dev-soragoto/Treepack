package manifest

import (
	"os"
	"path/filepath"
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
