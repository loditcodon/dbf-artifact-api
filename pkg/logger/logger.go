package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

// LogLevel represents the severity level of log messages.
type LogLevel int

// Log level constants defining message severity.
const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[LogLevel]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

// ParseLogLevel converts a string log level to its LogLevel constant.
func ParseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}

// Logger provides structured logging with level-based filtering and log rotation.
type Logger struct {
	debugLogger *log.Logger
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
	fatalLogger *log.Logger
	level       LogLevel
	mu          sync.RWMutex
}

var instance *Logger
var once sync.Once

// Init initializes the global logger instance with default configuration at INFO level.
func Init(logPath string) {
	once.Do(func() {
		instance = NewLogger(logPath, INFO)
	})
}

// InitWithConfig initializes the global logger instance with custom log rotation configuration.
func InitWithConfig(logPath string, level LogLevel, maxSize, maxBackups, maxAge int, compress bool) {
	once.Do(func() {
		instance = NewLoggerWithConfig(logPath, level, maxSize, maxBackups, maxAge, compress)
	})
}

// NewLogger creates a new logger instance with default log rotation settings.
func NewLogger(logPath string, level LogLevel) *Logger {
	return NewLoggerWithConfig(logPath, level, 10, 3, 28, true)
}

// NewLoggerWithConfig creates a new logger instance with custom log rotation configuration.
func NewLoggerWithConfig(logPath string, level LogLevel, maxSize, maxBackups, maxAge int, compress bool) *Logger {
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("cannot create directory log: %v", err)
	}

	if err := os.Chmod(dir, 0755); err != nil {
		log.Fatalf("cannot set privilege directory log: %v", err)
	}

	logFile := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   compress,
	}

	multiWriter := io.MultiWriter(os.Stdout, logFile)

	logger := &Logger{
		level: level,
	}

	flags := log.LstdFlags | log.Lshortfile

	logger.debugLogger = log.New(multiWriter, "[DEBUG] ", flags)
	logger.infoLogger = log.New(multiWriter, "[INFO] ", flags)
	logger.warnLogger = log.New(multiWriter, "[WARN] ", flags)
	logger.errorLogger = log.New(multiWriter, "[ERROR] ", flags)
	logger.fatalLogger = log.New(multiWriter, "[FATAL] ", flags)

	return logger
}

// SetLevel changes the minimum log level for filtering messages.
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel returns the current minimum log level.
func (l *Logger) GetLevel() LogLevel {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

func (l *Logger) shouldLog(level LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

// Debug logs a debug-level message.
func (l *Logger) Debug(v ...interface{}) {
	if l.shouldLog(DEBUG) {
		l.debugLogger.Output(2, fmt.Sprint(v...))
	}
}

// Debugf logs a formatted debug-level message.
func (l *Logger) Debugf(format string, v ...interface{}) {
	if l.shouldLog(DEBUG) {
		l.debugLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Info logs an info-level message.
func (l *Logger) Info(v ...interface{}) {
	if l.shouldLog(INFO) {
		l.infoLogger.Output(2, fmt.Sprint(v...))
	}
}

// Infof logs a formatted info-level message.
func (l *Logger) Infof(format string, v ...interface{}) {
	if l.shouldLog(INFO) {
		l.infoLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Warn logs a warning-level message.
func (l *Logger) Warn(v ...interface{}) {
	if l.shouldLog(WARN) {
		l.warnLogger.Output(2, fmt.Sprint(v...))
	}
}

// Warnf logs a formatted warning-level message.
func (l *Logger) Warnf(format string, v ...interface{}) {
	if l.shouldLog(WARN) {
		l.warnLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Error(v ...interface{}) {
	if l.shouldLog(ERROR) {
		l.errorLogger.Output(2, fmt.Sprint(v...))
	}
}

// Errorf logs a formatted error-level message.
func (l *Logger) Errorf(format string, v ...interface{}) {
	if l.shouldLog(ERROR) {
		l.errorLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Fatal logs a fatal-level message and exits the program.
func (l *Logger) Fatal(v ...interface{}) {
	if l.shouldLog(FATAL) {
		l.fatalLogger.Output(2, fmt.Sprint(v...))
		os.Exit(1)
	}
}

// Fatalf logs a formatted fatal-level message and exits the program.
func (l *Logger) Fatalf(format string, v ...interface{}) {
	if l.shouldLog(FATAL) {
		l.fatalLogger.Output(2, fmt.Sprintf(format, v...))
		os.Exit(1)
	}
}

// Global convenience functions

// Debug logs a debug-level message using the global logger instance.
func Debug(v ...interface{}) {
	if instance != nil {
		instance.Debug(v...)
	}
}

// Debugf logs a formatted debug-level message using the global logger instance.
func Debugf(format string, v ...interface{}) {
	if instance != nil {
		instance.Debugf(format, v...)
	}
}

// Info logs an info-level message using the global logger instance.
func Info(v ...interface{}) {
	if instance != nil {
		instance.Info(v...)
	}
}

// Infof logs a formatted info-level message using the global logger instance.
func Infof(format string, v ...interface{}) {
	if instance != nil {
		instance.Infof(format, v...)
	}
}

// Warn logs a warning-level message using the global logger instance.
func Warn(v ...interface{}) {
	if instance != nil {
		instance.Warn(v...)
	}
}

// Warnf logs a formatted warning-level message using the global logger instance.
func Warnf(format string, v ...interface{}) {
	if instance != nil {
		instance.Warnf(format, v...)
	}
}

// Error logs an error-level message using the global logger instance.
func Error(v ...interface{}) {
	if instance != nil {
		instance.Error(v...)
	}
}

// Errorf logs a formatted error-level message using the global logger instance.
func Errorf(format string, v ...interface{}) {
	if instance != nil {
		instance.Errorf(format, v...)
	}
}

// Fatal logs a fatal-level message and exits the program using the global logger instance.
func Fatal(v ...interface{}) {
	if instance != nil {
		instance.Fatal(v...)
	}
}

// Fatalf logs a formatted fatal-level message and exits the program using the global logger instance.
func Fatalf(format string, v ...interface{}) {
	if instance != nil {
		instance.Fatalf(format, v...)
	}
}

// SetLevel changes the minimum log level for the global logger instance.
func SetLevel(level LogLevel) {
	if instance != nil {
		instance.SetLevel(level)
	}
}

// GetLevel returns the current minimum log level of the global logger instance.
func GetLevel() LogLevel {
	if instance != nil {
		return instance.GetLevel()
	}
	return INFO
}
