package verify

import (
	"os"

	"treepack/internal/fsutil"
	"treepack/internal/manifest"
)

type Result struct {
	Label   string
	OK      bool
	Message string
}

// Run 根据校验配置检查输出目录中应存在或应缺失的路径。
func Run(config manifest.VerifyConfig, outputDir string) []Result {
	var results []Result
	for _, item := range config.Files {
		results = append(results, verifyPresent(outputDir, item, fsutil.EntryFile))
	}
	for _, item := range config.Dirs {
		results = append(results, verifyPresent(outputDir, item, fsutil.EntryDir))
	}
	for _, item := range config.Absent {
		path, err := fsutil.ResolveUnder(outputDir, item)
		if err != nil {
			results = append(results, Result{Label: "absent " + item, OK: false, Message: err.Error()})
			continue
		}
		_, err = os.Lstat(path)
		ok := os.IsNotExist(err)
		message := ""
		if err != nil && !os.IsNotExist(err) {
			message = err.Error()
		} else if !ok {
			message = "path should be absent"
		}
		results = append(results, Result{Label: "absent " + item, OK: ok, Message: message})
	}
	return results
}

// verifyPresent 校验指定输出路径存在且类型符合预期。
func verifyPresent(outputDir, item string, expected fsutil.EntryKind) Result {
	path, err := fsutil.ResolveUnder(outputDir, item)
	if err != nil {
		return Result{Label: item, OK: false, Message: err.Error()}
	}
	_, kind, err := fsutil.ValidateEntry(path)
	if err != nil {
		if os.IsNotExist(err) {
			if expected == fsutil.EntryDir {
				return Result{Label: item, OK: false, Message: "required directory is missing"}
			}
			return Result{Label: item, OK: false, Message: "required file is missing"}
		}
		return Result{Label: item, OK: false, Message: err.Error()}
	}
	if kind != expected {
		if expected == fsutil.EntryDir {
			return Result{Label: item, OK: false, Message: "required path is not a directory"}
		}
		return Result{Label: item, OK: false, Message: "required path is not a regular file"}
	}
	return Result{Label: item, OK: true}
}
