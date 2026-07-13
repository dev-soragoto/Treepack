package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"treepack/internal/fsutil"
)

var SupportedOps = map[string]bool{
	"cp": true, "cp_regex": true, "rm": true, "touch": true,
}

type OperationConfig struct {
	Op       string `toml:"op"`
	Required *bool  `toml:"required"`
	From     string `toml:"from"`
	To       string `toml:"to"`
	Path     string `toml:"path"`
	Regex    string `toml:"regex"`
}

// IsRequired 返回操作的最终必需性；未声明时继承调用方提供的包默认值。
func (o OperationConfig) IsRequired(defaultRequired bool) bool {
	if o.Required == nil {
		return defaultRequired
	}
	return *o.Required
}

type FS interface {
	CopyFile(src, dst string) error
	CopyTreeContents(src, dst string) error
	CopyExact(src, dst string) error
	Remove(path string) error
	EnsureParent(path string) error
}

type OperationResult struct {
	Op       string
	Label    string
	Required bool
	OK       bool
	Message  string
}

// ValidateOperation 校验单个构建操作所需字段和正则表达式。
func ValidateOperation(op OperationConfig, ctx string) error {
	if op.Op == "" {
		return fmt.Errorf("%s.op is required", ctx)
	}
	if !SupportedOps[op.Op] {
		return fmt.Errorf("%s.op unsupported: %s", ctx, op.Op)
	}
	switch op.Op {
	case "cp":
		if op.From == "" || op.To == "" {
			return fmt.Errorf("%s missing required key(s): from, to", ctx)
		}
	case "cp_regex":
		if op.From == "" || op.Regex == "" || op.To == "" {
			return fmt.Errorf("%s missing required key(s): from, regex, to", ctx)
		}
		if _, err := regexp.Compile(op.Regex); err != nil {
			return fmt.Errorf("%s.regex invalid regex: %w", ctx, err)
		}
	case "rm", "touch":
		if op.Path == "" {
			return fmt.Errorf("%s missing required key(s): path", ctx)
		}
	}
	return nil
}

// FailedRequired 返回操作结果是否代表必需步骤失败。
func (r OperationResult) FailedRequired() bool {
	return r.Required && !r.OK
}

// Run 在工作目录内执行单个操作并返回结构化结果。
func Run(config OperationConfig, workDir string, fs FS, defaultRequired bool) OperationResult {
	label := Label(config)
	required := config.IsRequired(defaultRequired)
	if err := dispatch(config, workDir, fs); err != nil {
		return OperationResult{Op: config.Op, Label: label, Required: required, OK: false, Message: err.Error()}
	}
	return OperationResult{Op: config.Op, Label: label, Required: required, OK: true}
}

// dispatch 根据操作类型分发到复制、删除或 touch 处理逻辑。
func dispatch(config OperationConfig, workDir string, fs FS) error {
	switch config.Op {
	case "cp":
		src, err := resolveOutput(config.From, workDir)
		if err != nil {
			return err
		}
		dst, err := resolveOutput(config.To, workDir)
		if err != nil {
			return err
		}
		return copyAny(src, dst, copyContents(config.From), fs)
	case "cp_regex":
		src, err := resolveOutput(config.From, workDir)
		if err != nil {
			return err
		}
		dst, err := resolveOutput(config.To, workDir)
		if err != nil {
			return err
		}
		return copyRegexOneLevel(src, config.Regex, dst, fs)
	case "rm":
		path, err := resolveOutput(config.Path, workDir)
		if err != nil {
			return err
		}
		return fs.Remove(path)
	case "touch":
		path, err := resolveOutput(config.Path, workDir)
		if err != nil {
			return err
		}
		if err := fs.EnsureParent(path); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE, 0o644)
		if err != nil {
			return err
		}
		return file.Close()
	default:
		return fmt.Errorf("unsupported op: %s", config.Op)
	}
}

// resolveOutput 将操作中的相对输出路径解析到工作目录下。
func resolveOutput(value, workDir string) (string, error) {
	return fsutil.ResolveUnder(workDir, value)
}

// copyAny 按源类型和目标状态执行文件、目录或目录内容复制。
func copyAny(src, dst string, contents bool, fs FS) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() && !isRealDir(info) {
		return fmt.Errorf("unsupported file type: %s", src)
	}
	if contents {
		if !isRealDir(info) {
			return fmt.Errorf("%s is not a directory", src)
		}
		if err := fsutil.RejectCopyOverlap(src, dst); err != nil {
			return err
		}
		return copyDirContents(src, dst, fs)
	}
	if !isRealDir(info) {
		if dstInfo, err := os.Lstat(dst); err == nil && isRealDir(dstInfo) {
			dst = filepath.Join(dst, filepath.Base(src))
		}
		if err := fsutil.RejectCopyOverlap(src, dst); err != nil {
			return err
		}
		return copyFile(src, dst, fs)
	}
	if dstInfo, err := os.Lstat(dst); err == nil && isRealDir(dstInfo) {
		dst = filepath.Join(dst, filepath.Base(src))
	}
	if err := fsutil.RejectCopyOverlap(src, dst); err != nil {
		return err
	}
	return copyDir(src, dst, fs)
}

// copyRegexOneLevel 复制源目录第一层中匹配正则表达式的入口。
func copyRegexOneLevel(src, pattern, dst string, fs FS) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !isRealDir(info) {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := fsutil.RejectCopyOverlap(src, dst); err != nil {
		return err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	matched := false
	for _, entry := range entries {
		if !re.MatchString(entry.Name()) {
			continue
		}
		matched = true
		if err := copyToExactPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()), fs); err != nil {
			return err
		}
	}
	if !matched {
		return os.ErrNotExist
	}
	return nil
}

// copyDirContents 将目录内容逐项复制到目标目录。
func copyDirContents(src, dst string, fs FS) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyToExactPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()), fs); err != nil {
			return err
		}
	}
	return nil
}

// copyToExactPath 将单个文件系统入口复制到精确目标路径。
func copyToExactPath(src, dst string, fs FS) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() && !isRealDir(info) {
		return fmt.Errorf("unsupported file type: %s", src)
	}
	if err := fsutil.RejectCopyOverlap(src, dst); err != nil {
		return err
	}
	return fs.CopyExact(src, dst)
}

// copyDir 通过文件系统适配器复制完整目录。
func copyDir(src, dst string, fs FS) error {
	return fs.CopyExact(src, dst)
}

// copyFile 通过文件系统适配器复制单个文件。
func copyFile(src, dst string, fs FS) error {
	return fs.CopyFile(src, dst)
}

// copyContents 判断复制语义是否表示复制目录内容而非目录本身。
func copyContents(value string) bool {
	normalized := filepath.ToSlash(value)
	return normalized == "." || strings.HasSuffix(normalized, "/.")
}

// isRealDir 判断文件信息是否表示真实目录。
func isRealDir(info os.FileInfo) bool {
	return info.IsDir() && info.Mode()&os.ModeType == os.ModeDir
}

// Label 为操作生成用于日志和报告的人类可读标签。
func Label(config OperationConfig) string {
	switch config.Op {
	case "cp":
		return fmt.Sprintf("%s %s -> %s", config.Op, config.From, config.To)
	case "cp_regex":
		return fmt.Sprintf("%s %s / %s -> %s", config.Op, config.From, config.Regex, config.To)
	case "rm", "touch":
		return fmt.Sprintf("%s %s", config.Op, config.Path)
	default:
		return config.Op
	}
}
