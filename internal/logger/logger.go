// Package logger 提供统一的分级日志输出，替代散落的 fmt.Printf/log.Printf 调用。
package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Level 表示日志级别。
type Level int

const (
	// Debug 级别：详细的调试信息。
	Debug Level = iota
	// Info 级别：常规进度信息。
	Info
	// Warn 级别：警告信息。
	Warn
	// Error 级别：错误信息。
	Error
)

var (
	level           = Info
	out   io.Writer = os.Stderr
)

const (
	colorReset = "\033[0m"
	colorDebug = "\033[90m"
	colorInfo  = "\033[32m"
	colorWarn  = "\033[33m"
	colorError = "\033[31m"
)

// SetLevel 设置全局日志级别。
func SetLevel(l Level) {
	if l < Debug || l > Error {
		l = Info
	}
	level = l
}

// ParseLevel 将字符串解析为日志级别，无法识别时返回 (Info, false)。
func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return Debug, true
	case "info":
		return Info, true
	case "warn", "warning":
		return Warn, true
	case "error":
		return Error, true
	default:
		return Info, false
	}
}

// String 返回级别的字符串表示。
func (l Level) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	default:
		return "INFO"
	}
}

// SetOutput 设置日志输出目标（默认 os.Stderr），便于测试。
func SetOutput(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	out = w
}

// Debugf 输出调试级别日志。
func Debugf(format string, args ...interface{}) {
	logAt(Debug, format, args...)
}

// Infof 输出信息级别日志。
func Infof(format string, args ...interface{}) {
	logAt(Info, format, args...)
}

// Warnf 输出警告级别日志。
func Warnf(format string, args ...interface{}) {
	logAt(Warn, format, args...)
}

// Errorf 输出错误级别日志。
func Errorf(format string, args ...interface{}) {
	logAt(Error, format, args...)
}

// Fatalf 输出错误级别日志并以非零状态退出。
func Fatalf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(out, "%s[ERR]%s %s%s\n", colorError, colorReset, msg, colorReset)
	os.Exit(1)
}

func logAt(l Level, format string, args ...interface{}) {
	if l < level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	color := colorInfo
	switch l {
	case Debug:
		color = colorDebug
	case Warn:
		color = colorWarn
	case Error:
		color = colorError
	}
	fmt.Fprintf(out, "%s[%s]%s %s%s\n", color, l.String(), colorReset, msg, colorReset)
}
