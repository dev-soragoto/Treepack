package build

import (
	"fmt"
	"os"
	"path/filepath"

	"treepack/internal/archive"
	"treepack/internal/fsutil"
	"treepack/internal/logging"
	"treepack/internal/manifest"
	"treepack/internal/ops"
	"treepack/internal/report"
)

// createLayout 在输出目录中创建清单声明的目录布局。
func createLayout(m *manifest.Manifest, outputDir string) error {
	for _, dir := range m.Layout.Dirs {
		layoutDir, err := fsutil.ResolveUnder(outputDir, dir)
		if err != nil {
			return fmt.Errorf("invalid layout dir %q: %w", dir, err)
		}
		if err := os.MkdirAll(layoutDir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// copyResources 将清单中的资源目录复制到构建输出并记录操作结果。
func copyResources(m *manifest.Manifest, sourceDir, outputDir string, fs fsAdapter, rep *report.BuildReport, logger *logging.Logger) {
	if m.Resources.Copy == "" {
		return
	}
	src, err := fsutil.ResolveUnder(sourceDir, m.Resources.Copy)
	result := ops.OperationResult{Op: "cp", Label: "resources " + m.Resources.Copy + " -> .", Required: true, OK: true}
	if logger != nil && err == nil {
		logger.Info("copying resources: %s -> %s", src, outputDir)
	}
	if err != nil {
		result.OK = false
		result.Message = err.Error()
	} else if _, kind, err := fsutil.ValidateEntry(src); err != nil || kind != fsutil.EntryDir {
		msg := src
		if err != nil {
			msg = err.Error()
		}
		result.OK = false
		result.Message = msg
	} else if err := fs.CopyTreeContents(src, outputDir); err != nil {
		result.OK = false
		result.Message = err.Error()
	}
	rep.AddOperation(result)
	logOperation(result, logger)
}

// writeReports 生成构建报告文件并写入输出目录。
func writeReports(m *manifest.Manifest, outputDir string, rep *report.BuildReport, version string) error {
	buildInfo, err := fsutil.ResolveUnder(outputDir, m.Build.BuildInfo)
	if err != nil {
		return fmt.Errorf("invalid build.build_info: %w", err)
	}
	if err := fsutil.EnsureParent(buildInfo); err != nil {
		return err
	}
	return os.WriteFile(buildInfo, []byte(report.BuildInfo(rep, version)), 0o644)
}

// cleanDir 清空指定目录并重新创建它。
func cleanDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

// finalizeOutput writes staged output to the final output directory and optionally archives it.
func finalizeOutput(m *manifest.Manifest, sourceDir, outputDir, workBase, runDir, stagedOutput string, fs fsAdapter, rep *report.BuildReport, logger *logging.Logger, rawArchive bool) (*report.BuildReport, error) {
	archivePath := ""
	if m.Build.Archive != "" {
		archiveName, err := renderTemplate(m.Build.Archive, m.Pack.Name, m.Pack.Version)
		if err != nil {
			return rep, fmt.Errorf("invalid build.archive: %w", err)
		}
		archivePath, err = resolveConfiguredPath(filepath.Dir(m.Path), archiveName)
		if err != nil {
			return rep, fmt.Errorf("invalid build.archive: %w", err)
		}
		if err := validateArchivePath(archivePath, sourceDir, outputDir, workBase, runDir, m.Path); err != nil {
			return rep, err
		}
	}
	if logger != nil {
		logger.Info("writing final output: %s -> %s", stagedOutput, outputDir)
	}
	if err := cleanDir(outputDir); err != nil {
		return rep, err
	}
	if err := fs.CopyTreeContents(stagedOutput, outputDir); err != nil {
		return rep, err
	}
	if m.Build.Archive != "" {
		if logger != nil {
			logger.Info("creating archive: %s", archivePath)
		}
		if err := archive.MakeZip(outputDir, archivePath, archive.Options{Raw: rawArchive}); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
