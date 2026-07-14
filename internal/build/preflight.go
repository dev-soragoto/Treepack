package build

// preflightBuild runs the shared lexical validation first, then checks the
// filesystem-dependent archive constraints that Explain deliberately avoids.
func preflightBuild(ctx *BuildContext) error {
	if err := preflightExplain(ctx.Manifest, ctx.Paths); err != nil {
		return err
	}
	if ctx.Manifest.Build.Archive == "" {
		return nil
	}
	archivePath, err := resolveArchivePath(ctx.Manifest)
	if err != nil {
		return err
	}
	if err := validateArchivePath(archivePath, ctx.Paths.SourceRoot, ctx.Paths.OutputRoot, ctx.Paths.WorkBase, ctx.Paths.RunDir, ctx.Manifest.Path); err != nil {
		return err
	}
	ctx.ArchivePath = archivePath
	return nil
}
