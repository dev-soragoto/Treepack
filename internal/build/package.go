package build

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"treepack/internal/archive"
	"treepack/internal/fsutil"
	"treepack/internal/logging"
	"treepack/internal/manifest"
	"treepack/internal/ops"
	"treepack/internal/report"
	"treepack/internal/source"
)

type installRequest struct {
	Package     manifest.PackageConfig
	SourceDir   string
	DownloadDir string
	PackageDir  string
	Token       string
	Proxy       string
	Retries     int
	Progress    io.Writer
	HTTPClient  *http.Client
	FS          fsAdapter
	Record      *report.PackageRecord
	Report      *report.BuildReport
	Logger      *logging.Logger
}

// installPackage 解析包资源、安装资源并执行该包的构建步骤。
func installPackage(req installRequest) (string, error) {
	pkg := req.Package
	fs := req.FS
	packageDir := req.PackageDir
	packageOutputDir := filepath.Join(packageDir, "output")
	if err := os.MkdirAll(packageOutputDir, 0o755); err != nil {
		return "", err
	}
	assetConfigs := packageAssets(pkg)
	assetRequests := make([]source.AssetRequest, 0, len(assetConfigs))
	for _, assetConfig := range assetConfigs {
		assetRequests = append(assetRequests, source.AssetRequest{Pattern: assetConfig.Asset, SHA256: assetConfig.SHA256})
	}
	resolvedAssets, err := source.Resolve(source.ResolveRequest{
		Source:      pkg.Source,
		Assets:      assetRequests,
		Root:        req.SourceDir,
		DownloadDir: req.DownloadDir,
		GitHubToken: req.Token,
		Proxy:       req.Proxy,
		Retries:     req.Retries,
		Progress:    req.Progress,
		Hasher:      fs,
		HTTPClient:  req.HTTPClient,
	})
	if err != nil {
		return "", err
	}
	extractNames := map[string]string{}
	for i, assetConfig := range assetConfigs {
		resolved := resolvedAssets[i]
		req.Record.Assets = append(req.Record.Assets, resolved)
		if err := installAsset(resolved, assetConfig, packageDir, packageOutputDir, extractNames, fs, req.Logger); err != nil {
			return "", err
		}
	}
	for _, step := range pkg.Steps {
		result := ops.Run(step, packageDir, fs, pkg.IsRequired())
		req.Report.AddOperation(result)
		logOperation(result, req.Logger)
		if result.FailedRequired() {
			return "", errors.New(result.Message)
		}
	}
	return packageOutputDir, nil
}

// packageAssets 统一返回包的单资源或多资源配置列表。
func packageAssets(pkg manifest.PackageConfig) []manifest.AssetConfig {
	if len(pkg.Assets) > 0 {
		return pkg.Assets
	}
	return []manifest.AssetConfig{{
		Asset: pkg.Asset, Target: pkg.Target, Install: pkg.Install, SHA256: pkg.SHA256,
	}}
}

// installAsset 根据资源安装策略复制资源文件或解压 zip 资源。
func installAsset(resolved source.ResolvedAsset, assetConfig manifest.AssetConfig, packageDir, outputDir string, extractNames map[string]string, fs fsAdapter, logger *logging.Logger) error {
	if assetConfig.Install == "extract" {
		if resolved.Kind == "dir" {
			return fmt.Errorf("extract only supports zip assets: %s", resolved.AssetName)
		}
		if strings.ToLower(filepath.Ext(resolved.Path)) != ".zip" {
			return fmt.Errorf("extract only supports zip assets: %s", resolved.AssetName)
		}
		extractName := safeName(resolved.AssetName)
		if previous, ok := extractNames[extractName]; ok && previous != resolved.AssetName {
			return fmt.Errorf("extract staging name collision: %s and %s both map to %s", previous, resolved.AssetName, extractName)
		}
		extractNames[extractName] = resolved.AssetName
		extractDir, err := fsutil.ResolveUnder(filepath.Join(packageDir, "extract"), extractName)
		if err != nil {
			return err
		}
		if logger != nil {
			logger.Info("extracting asset: %s -> %s", resolved.Path, extractDir)
		}
		return archive.ExtractZip(resolved.Path, extractDir)
	}
	target := assetConfig.Target
	if target == "" {
		target = resolved.AssetName
	}
	if resolved.Kind == "dir" && target == "." {
		if logger != nil {
			logger.Key("installed asset contents: %s -> %s", resolved.Path, outputDir)
		}
		return fs.CopyTreeContents(resolved.Path, outputDir)
	}
	dest, err := fsutil.ResolveUnder(outputDir, target)
	if err != nil {
		return err
	}
	if resolved.Kind == "dir" {
		if err := fs.CopyExact(resolved.Path, dest); err != nil {
			return err
		}
	} else if target == "." {
		if err := fs.CopyFile(resolved.Path, filepath.Join(dest, resolved.AssetName)); err != nil {
			return err
		}
	} else if err := fs.CopyFile(resolved.Path, dest); err != nil {
		return err
	}
	if logger != nil {
		logger.Key("installed asset: %s -> %s", resolved.Path, dest)
	}
	return nil
}
