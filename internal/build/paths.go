package build

import (
	"fmt"
	"os"
	"path/filepath"

	"treepack/internal/fsutil"
	"treepack/internal/manifest"
)

type ResolvedPaths struct {
	Source   string
	Output   string
	WorkBase string
	RunDir   string
	KeepWork bool
}

// resolveBuildPaths 解析构建涉及的源目录、输出目录、工作目录和保留策略。
func resolveBuildPaths(m *manifest.Manifest, options Options) (sourceDir, outputDir, workDir string, keepWork bool, err error) {
	manifestDir := filepath.Dir(m.Path)
	sourceValue := m.Paths.Source
	sourceBase := manifestDir
	if options.Source != "" {
		sourceValue = options.Source
		sourceBase = "."
	}
	outputValue := m.Paths.Output
	outputBase := manifestDir
	if options.Output != "" {
		outputValue = options.Output
		outputBase = "."
	}
	workValue := m.Paths.Work
	workBasePath := manifestDir
	if options.WorkDir != "" {
		workValue = options.WorkDir
		workBasePath = "."
	}
	keepWork = m.Build.KeepWork || options.KeepWork
	if sourceValue == "" {
		return "", "", "", false, fmt.Errorf("paths.source is required")
	}
	if outputValue == "" {
		return "", "", "", false, fmt.Errorf("paths.output is required")
	}
	sourceDir, err = resolveConfiguredPath(sourceBase, sourceValue)
	if err != nil {
		return "", "", "", false, fmt.Errorf("invalid paths.source: %w", err)
	}
	outputDir, err = resolveConfiguredPath(outputBase, outputValue)
	if err != nil {
		return "", "", "", false, fmt.Errorf("invalid paths.output: %w", err)
	}
	if workValue != "" {
		workDir, err = resolveConfiguredPath(workBasePath, workValue)
		if err != nil {
			return "", "", "", false, fmt.Errorf("invalid paths.work: %w", err)
		}
	}
	_, kind, err := fsutil.ValidateEntry(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", "", false, fmt.Errorf("paths.source must exist: %s", sourceDir)
		}
		return "", "", "", false, fmt.Errorf("invalid paths.source %s: %w", sourceDir, err)
	}
	if kind != fsutil.EntryDir {
		return "", "", "", false, fmt.Errorf("paths.source is not a directory: %s", sourceDir)
	}
	configPath := m.Path
	if err := rejectPathInside("config", configPath, "output", outputDir); err != nil {
		return "", "", "", false, err
	}
	if workDir != "" {
		if err := rejectPathInside("config", configPath, "work", workDir); err != nil {
			return "", "", "", false, err
		}
	}
	if err := rejectOverlap("paths.source", sourceDir, "paths.output", outputDir); err != nil {
		return "", "", "", false, err
	}
	if workDir != "" {
		if err := rejectOverlap("paths.source", sourceDir, "paths.work", workDir); err != nil {
			return "", "", "", false, err
		}
		if err := rejectOverlap("paths.output", outputDir, "paths.work", workDir); err != nil {
			return "", "", "", false, err
		}
	}
	return sourceDir, outputDir, workDir, keepWork, nil
}

// resolveConfiguredPath 将清单路径按指定基准目录解析为绝对路径。
func resolveConfiguredPath(baseDir, value string) (string, error) {
	path := filepath.FromSlash(value)
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Abs(path)
}

// createRunDir 在工作目录或系统临时目录中创建单次构建运行目录。
func createRunDir(workBase string) (string, error) {
	if workBase == "" {
		return os.MkdirTemp("", "treepack-*")
	}
	if err := os.MkdirAll(workBase, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(workBase, "run-*")
}

// rejectPathInside 拒绝某个路径位于指定基准目录内部。
func rejectPathInside(pathLabel, path, baseLabel, base string) error {
	inside, err := fsutil.Inside(base, path)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("%s file cannot be inside %s: %s", pathLabel, baseLabel, path)
	}
	return nil
}

// rejectOverlap 拒绝两个路径存在父子或相同路径关系。
func rejectOverlap(leftLabel, left, rightLabel, right string) error {
	overlap, err := fsutil.Overlap(left, right)
	if err != nil {
		return err
	}
	if overlap {
		return fmt.Errorf("%s and %s cannot overlap: %s / %s", leftLabel, rightLabel, left, right)
	}
	return nil
}

// validateArchivePath 校验归档输出路径不会覆盖目录或与构建关键路径重叠。
func validateArchivePath(archivePath, sourceDir, outputDir, workBase, runDir, manifestPath string) error {
	if info, err := os.Lstat(archivePath); err == nil && info.IsDir() {
		return fmt.Errorf("build.archive cannot be an existing directory: %s", archivePath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, item := range []struct {
		label string
		path  string
	}{
		{"paths.source", sourceDir},
		{"paths.output", outputDir},
		{"paths.work", workBase},
		{"work dir", runDir},
		{"manifest", manifestPath},
	} {
		if item.path == "" {
			continue
		}
		overlap, err := fsutil.Overlap(archivePath, item.path)
		if err != nil {
			return err
		}
		if overlap {
			return fmt.Errorf("build.archive cannot overlap %s: %s / %s", item.label, archivePath, item.path)
		}
	}
	return nil
}

// validateRunDir 校验运行目录不会与源目录或输出目录重叠。
func validateRunDir(runDir, sourceDir, outputDir string) error {
	if err := rejectOverlap("paths.source", sourceDir, "work dir", runDir); err != nil {
		return err
	}
	if err := rejectOverlap("paths.output", outputDir, "work dir", runDir); err != nil {
		return err
	}
	return nil
}
