package build

import (
	"fmt"
	"path/filepath"
	"strings"

	"treepack/internal/manifest"
	"treepack/internal/ops"
)

// Explain renders the build as an ordered list of static operations. It reads
// only the manifest: sources and archives are deliberately not inspected.
func Explain(options Options) (string, error) {
	m, err := manifest.Load(options.ConfigPath)
	if err != nil {
		return "", err
	}
	paths, err := resolveExplainPaths(m, options)
	if err != nil {
		return "", err
	}
	if err := preflightExplain(m, paths); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintln(&b, "Resolved paths")
	fmt.Fprintln(&b)
	writeResolvedPath(&b, "source", paths.SourceRoot, pathOrigin(options.Source, m.Paths.Source, "source"))
	fmt.Fprintln(&b)
	writeResolvedPath(&b, "output", paths.OutputRoot, pathOrigin(options.Output, m.Paths.Output, "output"))
	if paths.WorkBase != "" {
		fmt.Fprintln(&b)
		writeResolvedPath(&b, "work", paths.WorkBase, pathOrigin(options.WorkDir, m.Paths.Work, "work"))
	}
	if m.Build.Archive != "" {
		archivePath, _ := resolveArchivePath(m)
		fmt.Fprintln(&b)
		writeResolvedPath(&b, "archive", archivePath, "[build.archive] "+m.Build.Archive)
	}
	for packageIndex, pkg := range m.Packages {
		operationIndex := 0
		assets := packageAssets(pkg)
		for _, asset := range assets {
			operationIndex++
			if asset.Install == "extract" {
				writeExplainHeader(&b, packageIndex+1, operationIndex, "Extract asset: "+pkg.Name, pkg.IsRequired())
				name := explainAssetName(pkg, asset)
				fmt.Fprintln(&b, "package extract/")
				fmt.Fprintf(&b, "└── %s/\n", name)
				fmt.Fprintf(&b, "    └── <archive contents not inspected> -> %s\n", explainSource(pkg, asset))
				continue
			}
			writeExplainHeader(&b, packageIndex+1, operationIndex, "Install asset: "+pkg.Name, pkg.IsRequired())
			target := asset.Target
			if target == "" {
				target = explainAssetName(pkg, asset)
			}
			fmt.Fprintln(&b, "package output/")
			fmt.Fprintf(&b, "└── %s -> %s\n", explainDisplayPath(target), explainSource(pkg, asset))
		}
		for _, configured := range pkg.Steps {
			operationIndex++
			step := correctExtractStepPaths(configured)
			writeExplainHeader(&b, packageIndex+1, operationIndex, step.Op, step.IsRequired(pkg.IsRequired()))
			writeExplainStep(&b, pkg, configured, step)
		}
		operationIndex++
		writeExplainHeader(&b, packageIndex+1, operationIndex, "Merge package: "+pkg.Name, pkg.IsRequired())
		fmt.Fprintf(&b, "staged output/\n└── <package contents> -> package %03d-%s/output/\n", packageIndex+1, safeName(pkg.Name))
	}
	index := len(m.Packages) + 1
	fmt.Fprintf(&b, "\n[%d.1] layout\n\n", index)
	if len(m.Layout.Dirs) == 0 {
		fmt.Fprintln(&b, "staged output/\n└── <no configured directories>")
	} else {
		fmt.Fprintln(&b, "staged output/")
		for _, dir := range m.Layout.Dirs {
			fmt.Fprintf(&b, "└── %s/ -> generated: layout\n", explainDisplayPath(dir))
		}
	}
	fmt.Fprintf(&b, "\n[%d.2] verify\n\n", index)
	writeExplainVerify(&b, m)
	fmt.Fprintf(&b, "\n[%d.3] build_info\n\n", index)
	fmt.Fprintf(&b, "staged output/\n└── %s -> generated: build report\n", explainDisplayPath(m.Build.BuildInfo))
	next := 4
	if m.Build.Archive != "" {
		archivePath, _ := resolveArchivePath(m)
		fmt.Fprintf(&b, "\n[%d.%d] archive\n\n", index, next)
		fmt.Fprintf(&b, "%s -> staged output/\n", filepath.ToSlash(archivePath))
		next++
	}
	fmt.Fprintf(&b, "\n[%d.%d] publish\n\n", index, next)
	fmt.Fprintf(&b, "%s/ -> staged output/\n", filepath.ToSlash(paths.OutputRoot))
	fmt.Fprintln(&b, "\nNote: archive contents are not inspected; cp_regex matches extracted physical names.")
	return b.String(), nil
}

func resolveExplainPaths(m *manifest.Manifest, options Options) (ResolvedPaths, error) {
	manifestDir := filepath.Dir(m.Path)
	resolve := func(manifestValue, override, label string) (string, error) {
		value, base := manifestValue, manifestDir
		if override != "" {
			value, base = override, "."
		}
		if value == "" {
			return "", fmt.Errorf("paths.%s is required", label)
		}
		resolved, err := resolveConfiguredPath(base, value)
		if err != nil {
			return "", fmt.Errorf("invalid paths.%s: %w", label, err)
		}
		return resolved, nil
	}
	source, err := resolve(m.Paths.Source, options.Source, "source")
	if err != nil {
		return ResolvedPaths{}, err
	}
	output, err := resolve(m.Paths.Output, options.Output, "output")
	if err != nil {
		return ResolvedPaths{}, err
	}
	work := ""
	if m.Paths.Work != "" || options.WorkDir != "" {
		work, err = resolve(m.Paths.Work, options.WorkDir, "work")
		if err != nil {
			return ResolvedPaths{}, err
		}
	}
	return ResolvedPaths{SourceRoot: source, OutputRoot: output, WorkBase: work, KeepWork: m.Build.KeepWork || options.KeepWork}, nil
}

func preflightExplain(m *manifest.Manifest, paths ResolvedPaths) error {
	if staticInside(paths.OutputRoot, m.Path) {
		return fmt.Errorf("config file cannot be inside output: %s", m.Path)
	}
	if paths.WorkBase != "" && staticInside(paths.WorkBase, m.Path) {
		return fmt.Errorf("config file cannot be inside work: %s", m.Path)
	}
	if staticOverlap(paths.SourceRoot, paths.OutputRoot) {
		return fmt.Errorf("paths.source and paths.output cannot overlap: %s / %s", paths.SourceRoot, paths.OutputRoot)
	}
	if paths.WorkBase != "" && (staticOverlap(paths.SourceRoot, paths.WorkBase) || staticOverlap(paths.OutputRoot, paths.WorkBase)) {
		return fmt.Errorf("paths.work cannot overlap paths.source or paths.output")
	}
	for _, pkg := range m.Packages {
		pkgLabel := fmt.Sprintf("package(name: %q)", pkg.Name)
		if len(pkg.Assets) == 0 {
			if err := validateStaticRelative(pkg.Target, true); err != nil {
				return fmt.Errorf("invalid %s.target: %w", pkgLabel, err)
			}
		} else {
			for j, asset := range pkg.Assets {
				if err := validateStaticRelative(asset.Target, true); err != nil {
					return fmt.Errorf("invalid %s.assets[%d].target: %w", pkgLabel, j+1, err)
				}
			}
		}
		for j, step := range pkg.Steps {
			for _, item := range []struct{ field, value string }{{"from", step.From}, {"to", step.To}, {"path", step.Path}} {
				field, value := item.field, item.value
				if value != "" {
					if err := validateStaticRelative(value, true); err != nil {
						return fmt.Errorf("invalid %s.steps[%d].%s: %w", pkgLabel, j+1, field, err)
					}
				}
			}
		}
	}
	for _, group := range []struct {
		label string
		items []string
	}{
		{"layout.dirs", m.Layout.Dirs}, {"verify.files", m.Verify.Files},
		{"verify.dirs", m.Verify.Dirs}, {"verify.absent", m.Verify.Absent},
	} {
		for i, value := range group.items {
			if err := validateStaticRelative(value, true); err != nil {
				return fmt.Errorf("invalid %s[%d]: %w", group.label, i+1, err)
			}
		}
	}
	if err := validateStaticRelative(m.Build.BuildInfo, false); err != nil {
		return fmt.Errorf("invalid build.build_info: %w", err)
	}
	if m.Build.Archive != "" {
		archivePath, err := resolveArchivePath(m)
		if err != nil {
			return err
		}
		for _, other := range []string{paths.SourceRoot, paths.OutputRoot, paths.WorkBase, m.Path} {
			if other != "" && staticOverlap(archivePath, other) {
				return fmt.Errorf("build.archive cannot overlap a build path: %s / %s", archivePath, other)
			}
		}
	}
	return nil
}

func validateStaticRelative(value string, allowRoot bool) error {
	path := filepath.FromSlash(value)
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths are not allowed: %s", value)
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes base directory: %s", value)
	}
	if !allowRoot && (clean == "." || clean == "") {
		return fmt.Errorf("must name a file below the output directory")
	}
	return nil
}

func staticOverlap(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	rel, err := filepath.Rel(left, right)
	if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
		return true
	}
	rel, err = filepath.Rel(right, left)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func staticInside(base, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func writeResolvedPath(b *strings.Builder, label, resolved, origin string) {
	fmt.Fprintln(b, label)
	fmt.Fprintf(b, "└── %s -> %s\n", filepath.ToSlash(resolved), origin)
}

func pathOrigin(override, configured, name string) string {
	if override != "" {
		return "--" + strings.ReplaceAll(name, "work", "work-dir") + " " + override
	}
	return "[paths." + name + "] " + configured
}

func writeExplainHeader(b *strings.Builder, pkg, op int, title string, required bool) {
	kind := "optional"
	if required {
		kind = "required"
	}
	fmt.Fprintf(b, "\n[%d.%d] %s [%s]\n\n", pkg, op, title, kind)
}

func explainAssetName(pkg manifest.PackageConfig, asset manifest.AssetConfig) string {
	pattern := asset.Asset
	if pattern == "" {
		pattern = pkg.Asset
	}
	if pattern == "" {
		return "{asset}"
	}
	return "{asset:" + pattern + "}"
}

func explainSource(pkg manifest.PackageConfig, asset manifest.AssetConfig) string {
	if raw, ok := strings.CutPrefix(pkg.Source, "file:"); ok {
		return "file:" + raw
	}
	return pkg.Source + " / " + explainAssetName(pkg, asset)
}

func explainDisplayPath(value string) string {
	value = filepath.ToSlash(value)
	if value == "." {
		return "<contents>"
	}
	return value
}

func writeExplainStep(b *strings.Builder, pkg manifest.PackageConfig, original, corrected ops.OperationConfig) {
	switch corrected.Op {
	case "cp":
		fmt.Fprintf(b, "package %s\n└── %s -> package %s\n", explainStepArea(corrected.To), explainStepTarget(corrected.To), explainPackagePath(pkg, corrected.From))
	case "cp_regex":
		fmt.Fprintf(b, "package %s\n└── %s/<matches:%s> -> package %s/\n", explainStepArea(corrected.To), strings.TrimSuffix(explainStepTarget(corrected.To), "/"), corrected.Regex, explainPackagePath(pkg, corrected.From))
	case "rm":
		fmt.Fprintf(b, "package %s\n└── %s [remove]\n", explainStepArea(corrected.Path), explainStepTarget(corrected.Path))
	case "touch":
		fmt.Fprintf(b, "package %s\n└── %s -> generated: touch\n", explainStepArea(corrected.Path), explainStepTarget(corrected.Path))
	}
	for _, pair := range [][2]string{{original.From, corrected.From}, {original.To, corrected.To}, {original.Path, corrected.Path}} {
		if pair[0] != "" && pair[0] != pair[1] {
			fmt.Fprintf(b, "\nportable correction:\n└── %s -> %s\n", explainCorrectionSuffix(pair[0]), explainCorrectionSuffix(pair[1]))
		}
	}
}

func explainStepTarget(value string) string {
	value = filepath.ToSlash(value)
	if value == "output" || value == "extract" {
		return "<contents>"
	}
	for _, prefix := range []string{"output/", "extract/"} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return explainDisplayPath(value)
}

func explainPackagePath(pkg manifest.PackageConfig, value string) string {
	value = filepath.ToSlash(value)
	parts := strings.Split(value, "/")
	if len(parts) >= 2 && parts[0] == "extract" {
		for _, asset := range packageAssets(pkg) {
			if asset.Install == "extract" {
				parts[1] = explainAssetName(pkg, asset)
				break
			}
		}
		return strings.Join(parts, "/")
	}
	return value
}

func explainCorrectionSuffix(value string) string {
	parts := strings.Split(filepath.ToSlash(value), "/")
	if len(parts) > 2 && parts[0] == "extract" {
		return strings.Join(parts[2:], "/")
	}
	return value
}

func explainStepArea(value string) string {
	if strings.HasPrefix(filepath.ToSlash(value), "output/") || value == "output" {
		return "output/"
	}
	if strings.HasPrefix(filepath.ToSlash(value), "extract/") || value == "extract" {
		return "extract/"
	}
	return "root/"
}

func writeExplainVerify(b *strings.Builder, m *manifest.Manifest) {
	if len(m.Verify.Files)+len(m.Verify.Dirs)+len(m.Verify.Absent) == 0 {
		fmt.Fprintln(b, "staged output/\n└── <no configured checks>")
		return
	}
	fmt.Fprintln(b, "staged output/")
	for _, value := range m.Verify.Files {
		fmt.Fprintf(b, "└── %s [require file]\n", explainDisplayPath(value))
	}
	for _, value := range m.Verify.Dirs {
		fmt.Fprintf(b, "└── %s/ [require directory]\n", explainDisplayPath(value))
	}
	for _, value := range m.Verify.Absent {
		fmt.Fprintf(b, "└── %s [require absent]\n", explainDisplayPath(value))
	}
}
