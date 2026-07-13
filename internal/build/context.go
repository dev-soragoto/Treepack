package build

import (
	"net/http"

	"treepack/internal/logging"
	"treepack/internal/manifest"
	"treepack/internal/report"
)

type BuildContext struct {
	Manifest    *manifest.Manifest
	Paths       ResolvedPaths
	Options     Options
	Report      *report.BuildReport
	FS          fsAdapter
	Logger      *logging.Logger
	HTTPClient  *http.Client
	ArchivePath string
}
