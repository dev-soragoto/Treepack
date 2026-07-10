package logging

import (
	"fmt"
	"io"
	"os"
	"sync"
)

type Logger struct {
	out   io.Writer
	color bool
	mu    sync.Mutex
}

// New 创建写入指定输出的日志器，并根据输出能力决定是否启用颜色。
func New(out io.Writer) *Logger {
	return &Logger{out: out, color: supportsColor(out)}
}

// Info 写入普通信息日志。
func (l *Logger) Info(format string, args ...any) {
	l.printf("", format, args...)
}

// Key 写入表示成功或关键进度的绿色日志。
func (l *Logger) Key(format string, args ...any) {
	l.printf("\x1b[32m", format, args...)
}

// Warn 写入警告日志。
func (l *Logger) Warn(format string, args ...any) {
	l.printf("\x1b[33m", "warning: "+format, args...)
}

// Error 写入错误日志。
func (l *Logger) Error(format string, args ...any) {
	l.printf("\x1b[31m", "error: "+format, args...)
}

// printf 串行化格式化日志输出，并在支持时包裹 ANSI 颜色。
func (l *Logger) printf(color, format string, args ...any) {
	if l == nil || l.out == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.color && color != "" {
		fmt.Fprint(l.out, color)
		defer fmt.Fprint(l.out, "\x1b[0m")
	}
	fmt.Fprintf(l.out, format+"\n", args...)
}

// supportsColor 判断输出目标是否适合 ANSI 彩色日志。
func supportsColor(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
