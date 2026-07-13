package report

import (
	"fmt"
	"strings"
	"testing"

	"treepack/internal/manifest"
	"treepack/internal/ops"
	"treepack/internal/source"
)

// TestRenderers 验证构建报告会渲染关键字段。
func TestRenderers(t *testing.T) {
	rep := New(&manifest.Manifest{Pack: manifest.PackConfig{Name: "Pack", Version: "1"}})
	rep.Packages = []PackageRecord{{
		Package: manifest.PackageConfig{Name: "Pkg", Group: "core", Source: "github:o/r", Required: boolPtr(true)},
		OK:      true,
		Assets:  []source.ResolvedAsset{{Resolved: "v1", Requested: "latest", AssetName: "a.zip", URL: "https://example.com/a.zip", SHA256: "abc"}},
	}}
	info := BuildInfo(rep, "test")
	for _, want := range []string{"Name: treepack", "Version: test", "[core] Pkg - OK", "SHA256: abc", "Failures\n--------\nNone"} {
		if !strings.Contains(info, want) {
			t.Fatalf("build info missing %q:\n%s", want, info)
		}
	}
}

// TestBuildInfoSanitizesURLs 验证对应场景的行为。
func TestBuildInfoSanitizesURLs(t *testing.T) {
	rep := New(&manifest.Manifest{Pack: manifest.PackConfig{Name: "Pack"}})
	rep.Packages = []PackageRecord{{
		Package: manifest.PackageConfig{Name: "Pkg", Group: "core", Source: "url:https://user:pass@example.com/a.zip?token=secret#frag"},
		OK:      true,
		Assets: []source.ResolvedAsset{{
			Requested: "direct",
			Resolved:  "direct",
			AssetName: "a.zip",
			URL:       "https://user:pass@example.com/a.zip?token=secret#frag",
			SHA256:    "abc",
		}},
	}}
	info := BuildInfo(rep, "test")
	for _, bad := range []string{"user:pass", "token=secret", "#frag"} {
		if strings.Contains(info, bad) {
			t.Fatalf("build info leaked %q:\n%s", bad, info)
		}
	}
	if !strings.Contains(info, "Source: url:https://example.com/a.zip") || !strings.Contains(info, "URL: https://example.com/a.zip") {
		t.Fatalf("build info missing sanitized URLs:\n%s", info)
	}
}

// TestBuildInfoDoesNotPrintAbsoluteFileURL 验证对应场景的行为。
func TestBuildInfoDoesNotPrintAbsoluteFileURL(t *testing.T) {
	rep := New(&manifest.Manifest{Pack: manifest.PackConfig{Name: "Pack"}})
	rep.Packages = []PackageRecord{{
		Package: manifest.PackageConfig{Name: "Pkg", Group: "core", Source: "file:fixtures/a.bin"},
		OK:      true,
		Assets:  []source.ResolvedAsset{{Requested: "local", Resolved: "local", AssetName: "a.bin", URL: "file:///C:/Users/soragoto/secret/a.bin"}},
	}}
	info := BuildInfo(rep, "test")
	if strings.Contains(info, "C:/Users") || !strings.Contains(info, "URL: file:<local>") {
		t.Fatalf("unexpected file URL rendering:\n%s", info)
	}
}

func TestBuildInfoOmitsResolvedPaths(t *testing.T) {
	rep := New(&manifest.Manifest{Pack: manifest.PackConfig{Name: "Pack"}})
	rep.Paths.SourceRoot = `C:\Users\secret\source`
	rep.Paths.OutputRoot = `C:\Users\secret\output`
	rep.Paths.RunDir = `C:\Users\secret\work`
	rep.Operations = []ops.OperationResult{{
		Op: "cp", Label: "cp missing -> output", Required: false,
		Message: fmt.Sprintf(`CreateFile %s\packages\001\missing: file not found`, rep.Paths.RunDir),
	}}
	rep.Failures = []string{rep.Operations[0].Message}
	info := BuildInfo(rep, "test")
	for _, forbidden := range []string{"Resolved Paths", `C:\Users\secret`} {
		if strings.Contains(info, forbidden) {
			t.Fatalf("build info leaked %q:\n%s", forbidden, info)
		}
	}
}

func TestBuildInfoSanitizesAbsoluteFileSource(t *testing.T) {
	rep := New(&manifest.Manifest{Pack: manifest.PackConfig{Name: "Pack"}})
	rep.Packages = []PackageRecord{{
		Package: manifest.PackageConfig{Name: "Local", Source: `file:C:/Users/secret/asset.bin`},
		OK:      true,
	}}
	info := BuildInfo(rep, "test")
	if strings.Contains(info, "C:/Users/secret") || !strings.Contains(info, "Source: file:<local>") {
		t.Fatalf("absolute file source leaked:\n%s", info)
	}
}

// boolPtr 返回布尔值指针供测试配置使用。
func boolPtr(v bool) *bool {
	return &v
}
