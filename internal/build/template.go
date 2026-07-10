package build

import (
	"fmt"
	"strings"
)

// renderTemplate 渲染归档路径模板并拒绝未知占位符。
func renderTemplate(value, packName, packVersion string) (string, error) {
	replacements := map[string]string{
		"{pack.name}":      safeName(packName),
		"{pack.safe_name}": safeName(packName),
		"{pack.version}":   safeName(packVersion),
	}
	for placeholder, replacement := range replacements {
		value = strings.ReplaceAll(value, placeholder, replacement)
	}
	if start := strings.Index(value, "{"); start >= 0 {
		if end := strings.Index(value[start:], "}"); end >= 0 {
			return "", fmt.Errorf("unknown template placeholder: %s", value[start:start+end+1])
		}
		return "", fmt.Errorf("unknown template placeholder")
	}
	return value, nil
}

// safeName 将任意名称转换为适合文件名和临时目录名的安全字符串。
func safeName(value string) string {
	var builder strings.Builder
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-' {
			builder.WriteRune(ch)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}
