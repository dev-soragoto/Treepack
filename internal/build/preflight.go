package build

import (
	"fmt"
	"path/filepath"

	"treepack/internal/fsutil"
	"treepack/internal/ops"
)

// preflightBuild validates every configured output-relative path before any source is resolved.
func preflightBuild(ctx *BuildContext) error {
	for i, pkg := range ctx.Manifest.Packages {
		packageBase := filepath.Join(ctx.Paths.RunDir, "packages", fmt.Sprintf("%03d-%s", i+1, safeName(pkg.Name)))
		if len(pkg.Assets) == 0 {
			if err := validateRelativePath(fmt.Sprintf("packages[%d].target", i+1), filepath.Join(packageBase, "output"), pkg.Target, true); err != nil {
				return err
			}
		} else {
			for j, asset := range pkg.Assets {
				if err := validateRelativePath(fmt.Sprintf("packages[%d].assets[%d].target", i+1, j+1), filepath.Join(packageBase, "output"), asset.Target, true); err != nil {
					return err
				}
			}
		}
		for j, step := range pkg.Steps {
			if err := validateStepPaths(step, packageBase, fmt.Sprintf("packages[%d].steps[%d]", i+1, j+1)); err != nil {
				return err
			}
		}
	}
	for i, dir := range ctx.Manifest.Layout.Dirs {
		if err := validateRelativePath(fmt.Sprintf("layout.dirs[%d]", i+1), ctx.Paths.StagedOutput, dir, true); err != nil {
			return err
		}
	}
	for _, group := range []struct {
		label string
		items []string
	}{
		{"verify.files", ctx.Manifest.Verify.Files},
		{"verify.dirs", ctx.Manifest.Verify.Dirs},
		{"verify.absent", ctx.Manifest.Verify.Absent},
	} {
		for i, item := range group.items {
			if err := validateRelativePath(fmt.Sprintf("%s[%d]", group.label, i+1), ctx.Paths.StagedOutput, item, true); err != nil {
				return err
			}
		}
	}
	if err := validateRelativePath("build.build_info", ctx.Paths.StagedOutput, ctx.Manifest.Build.BuildInfo, false); err != nil {
		return err
	}
	if ctx.Manifest.Build.Archive != "" {
		archivePath, err := resolveArchivePath(ctx.Manifest)
		if err != nil {
			return err
		}
		if err := validateArchivePath(archivePath, ctx.Paths.SourceRoot, ctx.Paths.OutputRoot, ctx.Paths.WorkBase, ctx.Paths.RunDir, ctx.Manifest.Path); err != nil {
			return err
		}
		ctx.ArchivePath = archivePath
	}
	return nil
}

func validateStepPaths(step ops.OperationConfig, base, context string) error {
	for _, item := range []struct {
		field string
		value string
	}{
		{"from", step.From},
		{"to", step.To},
		{"path", step.Path},
	} {
		if item.value == "" {
			continue
		}
		if err := validateRelativePath(context+"."+item.field, base, item.value, true); err != nil {
			return err
		}
	}
	return nil
}

func validateRelativePath(label, base, value string, allowRoot bool) error {
	if value == "" {
		return nil
	}
	resolved, err := fsutil.ResolveUnder(base, value)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", label, err)
	}
	if !allowRoot && filepath.Clean(resolved) == filepath.Clean(base) {
		return fmt.Errorf("invalid %s: must name a file below the output directory", label)
	}
	return nil
}
