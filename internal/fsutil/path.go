package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveUnder 将相对路径解析到基准目录下，并拒绝绝对路径和目录逃逸。
func ResolveUnder(base, rel string) (string, error) {
	if rel == "" {
		return filepath.Clean(base), nil
	}
	path := filepath.FromSlash(rel)
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", rel)
	}
	target := filepath.Clean(filepath.Join(base, path))
	if err := CheckUnder(base, target); err != nil {
		return "", err
	}
	return target, nil
}

// CheckUnder 校验目标路径位于基准目录内部。
func CheckUnder(base, target string) error {
	inside, err := Inside(base, target)
	if err != nil {
		return err
	}
	if !inside {
		return fmt.Errorf("path escapes base directory: %s", target)
	}
	return nil
}

// Inside 判断目标路径是否位于基准目录内部或等于基准目录。
func Inside(base, target string) (bool, error) {
	cleanBase, err := Canonical(base)
	if err != nil {
		return false, err
	}
	cleanTarget, err := Canonical(target)
	if err != nil {
		return false, err
	}
	return insideClean(cleanBase, cleanTarget)
}

// Overlap 判断两个路径是否相同或互为父子路径。
func Overlap(left, right string) (bool, error) {
	leftInsideRight, err := Inside(right, left)
	if err != nil {
		return false, err
	}
	rightInsideLeft, err := Inside(left, right)
	if err != nil {
		return false, err
	}
	return leftInsideRight || rightInsideLeft, nil
}

// Canonical 返回路径的绝对规范形式，并解析已存在路径段中的符号链接。
func Canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	}
	clean := filepath.Clean(abs)
	var missing []string
	for {
		if resolved, err := filepath.EvalSymlinks(clean); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return filepath.Clean(abs), nil
		}
		missing = append(missing, filepath.Base(clean))
		if _, err := os.Lstat(parent); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		clean = parent
	}
}

// insideClean 在两个已规范化路径之间执行包含关系判断。
func insideClean(base, target string) (bool, error) {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}
