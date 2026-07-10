package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"treepack/internal/logging"
	"treepack/internal/manifest"
	"treepack/internal/ops"
	"treepack/internal/report"
	"treepack/internal/verify"
)

type Options struct {
	ConfigPath  string
	Source      string
	Output      string
	WorkDir     string
	KeepWork    bool
	RawArchive  bool
	Retries     int
	GitHubToken string
	Proxy       string
	Version     string
	Logger      *logging.Logger
	Progress    io.Writer
}

// Build 根据配置和命令行覆盖项完成资源解析、安装、校验、发布和归档。
func Build(options Options) (*report.BuildReport, error) {
	if options.Retries == 0 {
		options.Retries = 3
	}
	if options.Retries < 1 {
		return nil, fmt.Errorf("download retries must be at least 1")
	}
	logger := options.Logger
	if logger != nil {
		configAbs, _ := filepath.Abs(options.ConfigPath)
		logger.Info("loading manifest: %s", configAbs)
	}
	m, err := manifest.Load(options.ConfigPath)
	if err != nil {
		return nil, err
	}
	rep := report.New(m)
	sourceDir, outputDir, workBase, keepWork, err := resolveBuildPaths(m, options)
	if err != nil {
		return rep, err
	}
	m.Paths.Source = sourceDir
	m.Paths.Output = outputDir
	m.Paths.Work = workBase
	m.Build.KeepWork = keepWork
	if logger != nil {
		logger.Info("source dir: %s", sourceDir)
		logger.Info("output dir: %s", outputDir)
	}
	runDir, err := createRunDir(workBase)
	if err != nil {
		return rep, err
	}
	m.Paths.Work = runDir
	if logger != nil {
		logger.Info("work dir: %s", runDir)
	}
	if err := validateRunDir(runDir, sourceDir, outputDir); err != nil {
		_ = os.RemoveAll(runDir)
		return rep, err
	}
	defer func() {
		if keepWork {
			if logger != nil {
				logger.Warn("keeping work dir: %s", runDir)
			}
			return
		}
		if logger != nil {
			logger.Info("cleaning work dir: %s", runDir)
		}
		_ = os.RemoveAll(runDir)
	}()
	stagedOutput := filepath.Join(runDir, "output")
	if err := cleanDir(stagedOutput); err != nil {
		return rep, err
	}
	if usesGitHub(m) && options.GitHubToken == "" && logger != nil {
		logger.Info("github token not configured; using anonymous requests")
	}
	fs := fsAdapter{}
	for i, pkg := range m.Packages {
		if logger != nil {
			logger.Info("resolving package: %s", pkg.Name)
		}
		record := report.PackageRecord{Package: pkg}
		packageDir := filepath.Join(runDir, "packages", fmt.Sprintf("%03d-%s", i+1, safeName(pkg.Name)))
		downloadDir := filepath.Join(runDir, "downloads", fmt.Sprintf("%03d-%s", i+1, safeName(pkg.Name)))
		if logger != nil {
			logger.Info("package staging: %s", packageDir)
			logger.Info("package downloads: %s", downloadDir)
		}
		packageOutputDir, err := installPackage(pkg, sourceDir, downloadDir, packageDir, options.GitHubToken, options.Proxy, options.Retries, options.Progress, fs, &record, rep, logger)
		if err != nil {
			record.OK = false
			record.Message = err.Error()
			rep.AddPackageFailure(pkg, err.Error())
			rep.Packages = append(rep.Packages, record)
			if pkg.IsRequired() {
				if logger != nil {
					logger.Error("required package failed: %s: %s", pkg.Name, err)
				}
				break
			}
			if logger != nil {
				logger.Warn("optional package failed: %s: %s", pkg.Name, err)
			}
			continue
		}
		record.OK = true
		rep.Packages = append(rep.Packages, record)
		if err := fs.CopyTreeContents(packageOutputDir, stagedOutput); err != nil {
			return rep, err
		}
		if logger != nil {
			logger.Key("merged package: %s -> %s", packageOutputDir, stagedOutput)
		}
	}
	if !rep.HasRequiredFailures() {
		if err := createLayout(m, stagedOutput); err != nil {
			return rep, err
		}
		copyResources(m, sourceDir, stagedOutput, fs, rep, logger)
		if logger != nil {
			logger.Info("running verification")
		}
		rep.Verification = verify.Run(m.Verify, stagedOutput)
		for _, result := range rep.Verification {
			if !result.OK {
				rep.Failures = append(rep.Failures, fmt.Sprintf("verification failed: %s: %s", result.Label, result.Message))
				if logger != nil {
					logger.Error("verification failed: %s: %s", result.Label, result.Message)
				}
			} else if logger != nil {
				logger.Key("verified: %s", result.Label)
			}
		}
	}
	if logger != nil {
		logger.Info("writing reports")
	}
	if err := writeReports(m, stagedOutput, rep, options.Version); err != nil {
		return rep, err
	}
	if rep.HasRequiredFailures() {
		return rep, fmt.Errorf("required build step failed")
	}
	return finalizeOutput(m, sourceDir, outputDir, workBase, runDir, stagedOutput, fs, rep, logger, options.RawArchive)
}

// usesGitHub 判断清单中是否包含 GitHub release 类型的包来源。
func usesGitHub(m *manifest.Manifest) bool {
	for _, pkg := range m.Packages {
		if strings.HasPrefix(pkg.Source, "github:") {
			return true
		}
	}
	return false
}

// logOperation 按操作结果级别写入日志。
func logOperation(result ops.OperationResult, logger *logging.Logger) {
	if logger == nil {
		return
	}
	if result.OK {
		logger.Key("operation ok: %s", result.Label)
	} else if result.Required {
		logger.Error("required operation failed: %s: %s", result.Label, result.Message)
	} else {
		logger.Warn("optional operation failed: %s: %s", result.Label, result.Message)
	}
}
