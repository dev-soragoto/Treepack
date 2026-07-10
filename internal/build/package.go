package build

import (
	"errors"
	"fmt"
	"io"
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

// installPackage 解析包资源、安装资源并执行该包的构建步骤。
func installPackage(pkg manifest.PackageConfig, sourceDir, downloadDir, packageDir, token, proxy string, retries int, progress io.Writer, fs fsAdapter, record *report.PackageRecord, rep *report.BuildReport, logger *logging.Logger) (string, error) {
	packageOutputDir := filepath.Join(packageDir, "output")
	if err := os.MkdirAll(packageOutputDir, 0o755); err != nil {
		return "", err
	}
	assetConfigs := packageAssets(pkg)
	patterns := make([]string, 0, len(assetConfigs))
	for _, assetConfig := range assetConfigs {
		patterns = append(patterns, assetConfig.Asset)
	}
	resolvedAssets, err := source.ResolveAssets(pkg.Source, patterns, sourceDir, downloadDir, token, proxy, retries, progress, fs)
	if err != nil {
		return "", err
	}
	extractNames := map[string]string{}
	for i, assetConfig := range assetConfigs {
		resolved := resolvedAssets[i]
		record.Assets = append(record.Assets, resolved)
		if err := installAsset(resolved, assetConfig, packageDir, packageOutputDir, extractNames, fs, logger); err != nil {
			return "", err
		}
	}
	for _, step := range pkg.Steps {
		result := ops.Run(step, packageDir, fs)
		rep.AddOperation(result)
		logOperation(result, logger)
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
		Asset: pkg.Asset, Target: pkg.Target, Install: pkg.Install,
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
