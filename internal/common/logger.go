package common

import (
	"fmt"
	"log"
	"os"
	"sync"
)

// LogLevel 日志级别
type LogLevel int

const (
	// LevelDebug 调试级别
	LevelDebug LogLevel = iota
	// LevelInfo 信息级别
	LevelInfo
	// LevelError 错误级别
	LevelError
)

// Logger 日志记录器
type Logger struct {
	mu       sync.Mutex
	level    LogLevel
	verbose  bool
	logger   *log.Logger
}

var (
	// DefaultLogger 默认日志记录器
	DefaultLogger *Logger
)

func init() {
	DefaultLogger = NewLogger(false)
}

// NewLogger 创建新的日志记录器
func NewLogger(verbose bool) *Logger {
	level := LevelInfo
	if verbose {
		level = LevelDebug
	}

	return &Logger{
		level:   level,
		verbose: verbose,
		logger:  log.New(os.Stdout, "", log.LstdFlags),
	}
}

// SetVerbose 设置详细模式
func (l *Logger) SetVerbose(verbose bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.verbose = verbose
	if verbose {
		l.level = LevelDebug
	} else {
		l.level = LevelInfo
	}
}

// Debug 输出调试日志
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level <= LevelDebug {
		l.log("DEBUG", format, v...)
	}
}

// Info 输出信息日志
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level <= LevelInfo {
		l.log("INFO", format, v...)
	}
}

// Error 输出错误日志
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level <= LevelError {
		l.log("ERROR", format, v...)
	}
}

// log 内部日志输出方法
func (l *Logger) log(level, format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, v...)
	l.logger.Printf("[%s] %s", level, msg)
}

// 全局便捷函数

// SetVerbose 设置默认日志记录器的详细模式
func SetVerbose(verbose bool) {
	DefaultLogger.SetVerbose(verbose)
}

// Debug 使用默认日志记录器输出调试日志
func Debug(format string, v ...interface{}) {
	DefaultLogger.Debug(format, v...)
}

// Info 使用默认日志记录器输出信息日志
func Info(format string, v ...interface{}) {
	DefaultLogger.Info(format, v...)
}

// Error 使用默认日志记录器输出错误日志
func Error(format string, v ...interface{}) {
	DefaultLogger.Error(format, v...)
}
