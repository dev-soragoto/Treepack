package build

import (
	"fmt"
	"os"
	"path/filepath"

	"treepack/internal/archive"
	"treepack/internal/fsutil"
	"treepack/internal/manifest"
	"treepack/internal/report"
)

var makeArchive = archive.MakeZip

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

// writeReports 生成构建报告文件并写入输出目录。
func writeReports(ctx *BuildContext) error {
	buildInfo, err := fsutil.ResolveUnder(ctx.Paths.StagedOutput, ctx.Manifest.Build.BuildInfo)
	if err != nil {
		return fmt.Errorf("invalid build.build_info: %w", err)
	}
	if err := fsutil.EnsureParent(buildInfo); err != nil {
		return err
	}
	return os.WriteFile(buildInfo, []byte(report.BuildInfo(ctx.Report, ctx.Options.Version)), 0o644)
}

// cleanDir 清空指定目录并重新创建它。
func cleanDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

// finalizeOutput writes staged output to the final output directory and optionally archives it.
func finalizeOutput(ctx *BuildContext) (*report.BuildReport, error) {
	m := ctx.Manifest
	tempArchivePath := ""
	if m.Build.Archive != "" {
		archivePath := ctx.ArchivePath
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			return ctx.Report, err
		}
		temp, err := os.CreateTemp(filepath.Dir(archivePath), ".treepack-archive-*")
		if err != nil {
			return ctx.Report, err
		}
		tempArchivePath = temp.Name()
		if err := temp.Close(); err != nil {
			_ = os.Remove(tempArchivePath)
			return ctx.Report, err
		}
		if err := os.Remove(tempArchivePath); err != nil {
			return ctx.Report, err
		}
		defer func() { _ = os.Remove(tempArchivePath) }()
		if ctx.Logger != nil {
			ctx.Logger.Info("creating archive: %s", archivePath)
		}
		if err := makeArchive(ctx.Paths.StagedOutput, tempArchivePath, archive.Options{Raw: ctx.Options.RawArchive}); err != nil {
			return ctx.Report, err
		}
	}
	if ctx.Logger != nil {
		ctx.Logger.Info("writing final output: %s -> %s", ctx.Paths.StagedOutput, ctx.Paths.OutputRoot)
	}
	if err := cleanDir(ctx.Paths.OutputRoot); err != nil {
		return ctx.Report, err
	}
	if err := ctx.FS.CopyTreeContents(ctx.Paths.StagedOutput, ctx.Paths.OutputRoot); err != nil {
		return ctx.Report, err
	}
	if m.Build.Archive != "" {
		if err := os.Remove(ctx.ArchivePath); err != nil && !os.IsNotExist(err) {
			return ctx.Report, err
		}
		if err := os.Rename(tempArchivePath, ctx.ArchivePath); err != nil {
			return ctx.Report, err
		}
	}
	return ctx.Report, nil
}
