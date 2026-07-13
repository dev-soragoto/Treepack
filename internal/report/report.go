package report

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"treepack/internal/manifest"
	"treepack/internal/ops"
	"treepack/internal/source"
	"treepack/internal/verify"
)

type PackageRecord struct {
	Package manifest.PackageConfig
	Assets  []source.ResolvedAsset
	OK      bool
	Message string
}

type BuildReport struct {
	Manifest *manifest.Manifest
	Paths    struct {
		SourceRoot   string
		OutputRoot   string
		WorkBase     string
		RunDir       string
		StagedOutput string
		KeepWork     bool
	}
	Packages     []PackageRecord
	Operations   []ops.OperationResult
	Verification []verify.Result
	Failures     []string
}

// New 创建关联清单的空构建报告。
func New(m *manifest.Manifest) *BuildReport {
	return &BuildReport{Manifest: m}
}

// AddOperation 记录操作结果，并把失败操作追加到失败摘要。
func (r *BuildReport) AddOperation(result ops.OperationResult) {
	r.Operations = append(r.Operations, result)
	if !result.OK {
		prefix := "optional"
		if result.Required {
			prefix = "required"
		}
		r.Failures = append(r.Failures, fmt.Sprintf("%s operation failed: %s: %s", prefix, result.Label, result.Message))
	}
}

// AddPackageFailure 记录包安装失败并按必需性写入失败摘要。
func (r *BuildReport) AddPackageFailure(pkg manifest.PackageConfig, message string) {
	prefix := "optional"
	if pkg.IsRequired() {
		prefix = "required"
	}
	r.Failures = append(r.Failures, fmt.Sprintf("%s package failed: %s: %s", prefix, pkg.Name, message))
}

// HasRequiredFailures 判断报告中是否存在必需包或校验失败。
func (r *BuildReport) HasRequiredFailures() bool {
	for _, record := range r.Packages {
		if record.Package.IsRequired() && !record.OK {
			return true
		}
	}
	for _, result := range r.Verification {
		if !result.OK {
			return true
		}
	}
	return false
}

// BuildInfo 将构建报告渲染为 BUILD_INFO 文本内容。
func BuildInfo(r *BuildReport, version string) string {
	m := r.Manifest
	lines := []string{
		"Pack",
		"----",
		"Name: " + m.Pack.Name,
		"Version: " + m.Pack.Version,
		"",
		"Builder",
		"-------",
		"Name: treepack",
		"Version: " + version,
		"",
		"Resolved Packages",
		"-----------------",
	}
	if len(r.Packages) == 0 {
		lines = append(lines, "None")
	}
	for _, record := range r.Packages {
		status := "FAIL"
		if record.OK {
			status = "OK"
		}
		pkg := record.Package
		lines = append(lines, fmt.Sprintf("[%s] %s - %s", pkg.Group, pkg.Name, status))
		lines = append(lines, "  Source: "+sanitizeSource(pkg.Source))
		lines = append(lines, fmt.Sprintf("  Required: %t", pkg.IsRequired()))
		if record.Message != "" {
			lines = append(lines, "  Message: "+record.Message)
		}
		for _, asset := range record.Assets {
			lines = append(lines, "  Requested: "+asset.Requested)
			lines = append(lines, "  Resolved: "+asset.Resolved)
			lines = append(lines, "  Asset: "+asset.AssetName)
			lines = append(lines, "  URL: "+sanitizeAssetURL(asset.URL))
			if asset.SHA256 != "" {
				lines = append(lines, "  SHA256: "+asset.SHA256)
			}
		}
	}
	lines = append(lines, "", "Operations", "----------")
	if len(r.Operations) == 0 {
		lines = append(lines, "None")
	}
	for _, result := range r.Operations {
		if result.OK {
			lines = append(lines, "OK: "+result.Label)
			continue
		}
		kind := "optional"
		if result.Required {
			kind = "required"
		}
		lines = append(lines, fmt.Sprintf("FAIL(%s): %s: %s", kind, result.Label, result.Message))
	}
	lines = append(lines, "", "Verification", "------------")
	if len(r.Verification) == 0 {
		lines = append(lines, "None")
	}
	for _, result := range r.Verification {
		if result.OK {
			lines = append(lines, "OK: "+result.Label)
		} else {
			lines = append(lines, "FAIL: "+result.Label+": "+result.Message)
		}
	}
	lines = append(lines, "", "Failures", "--------")
	if len(r.Failures) == 0 {
		lines = append(lines, "None")
	} else {
		lines = append(lines, r.Failures...)
	}
	return sanitizeResolvedPaths(strings.Join(lines, "\n")+"\n", r)
}

// sanitizeSource 清理来源字符串中不适合写入报告的 URL 敏感部分。
func sanitizeSource(value string) string {
	if raw, ok := strings.CutPrefix(value, "file:"); ok {
		if filepath.IsAbs(filepath.FromSlash(raw)) {
			return "file:<local>"
		}
		return value
	}
	raw, ok := strings.CutPrefix(value, "url:")
	if !ok {
		return value
	}
	return "url:" + sanitizeAssetURL(raw)
}

// sanitizeResolvedPaths removes local resolved paths from the published report while preserving them in memory.
func sanitizeResolvedPaths(value string, r *BuildReport) string {
	paths := []string{
		r.Paths.StagedOutput,
		r.Paths.RunDir,
		r.Paths.SourceRoot,
		r.Paths.OutputRoot,
		r.Paths.WorkBase,
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		value = strings.ReplaceAll(value, path, "<local-path>")
		slashed := filepath.ToSlash(path)
		if slashed != path {
			value = strings.ReplaceAll(value, slashed, "<local-path>")
		}
	}
	return value
}

// sanitizeAssetURL 移除 URL 中的用户信息、查询参数和片段。
func sanitizeAssetURL(value string) string {
	if strings.HasPrefix(value, "file:") {
		if strings.HasPrefix(value, "file:///") {
			return "file:<local>"
		}
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
