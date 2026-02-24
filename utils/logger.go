package utils

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dbfartifactapi/pkg/logger"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/gin-gonic/gin"
)

func createLoggerWriter(filePath string) (io.Writer, error) {
	dir := filepath.Dir(filePath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create log directory: %v", err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot chmod log directory: %v", err)
	}

	logFile := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}

	return logFile, nil
}

// Init structured logger with config from environment
func InitLogger(filePath string) {
	// Import config package to access config values
	// This will be improved when we integrate with main.go
	logger.Init(filePath)
	logger.Infof("Logger initialized at: %s", filePath)
}

// Init structured logger with full config
func InitLoggerWithConfig(filePath, level string, maxSize, maxBackups, maxAge int, compress bool) {
	logLevel := logger.ParseLogLevel(level)
	logger.InitWithConfig(filePath, logLevel, maxSize, maxBackups, maxAge, compress)
	logger.Infof("Logger initialized with level %s at: %s", level, filePath)
}

// Init logger default (write log console and file) - DEPRECATED
func InitLoggerLegacy(filePath string) {
	writer, err := createLoggerWriter(filePath)
	if err != nil {
		log.Fatalf("init logger error: %v", err)
	}
	multiWriter := io.MultiWriter(os.Stdout, writer)
	log.SetOutput(multiWriter)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// func InitLogger(logPath string) {
// 	dir := filepath.Dir(logPath)
// 	if err := os.MkdirAll(dir, 0755); err != nil {
// 		log.Fatalf("cannot create directory log: %v", err)
// 	}

// 	if err := os.Chmod(dir, 0755); err != nil {
// 		log.Fatalf("cannot set privilege directory log: %v", err)
// 	}

// 	logFile := &lumberjack.Logger{
// 		Filename:   logPath,
// 		MaxSize:    10, // MB
// 		MaxBackups: 3,
// 		MaxAge:     28, // ngày
// 		Compress:   true,
// 	}

// 	multiWriter := io.MultiWriter(os.Stdout, logFile)
// 	log.SetOutput(multiWriter)
// 	log.SetFlags(log.LstdFlags | log.Lshortfile)
// }

// Custom logger (write specific_log)
func NewCustomLogger(filePath string) (*log.Logger, error) {
	writer, err := createLoggerWriter(filePath)
	if err != nil {
		return nil, err
	}
	logger := log.New(writer, "", log.LstdFlags|log.Lshortfile)
	return logger, nil
}

var policyLogger *log.Logger
var once sync.Once

// GetPolicyLogger returns a singleton logger instance for policy exception logging.
func GetPolicyLogger() *log.Logger {
	once.Do(func() {
		l, err := NewCustomLogger("/var/log/dbf/policy_exception.log")
		if err != nil {
			log.Printf("[ERROR] Cannot init custom policy logger: %v", err)
		} else {
			policyLogger = l
		}
	})
	return policyLogger
}

// Enhanced structured middleware log
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		elapsed := time.Since(start)
		status := c.Writer.Status()

		// Log based on status code level
		if status >= 500 {
			logger.Errorf("HTTP %s %s - Status: %d, Duration: %v, IP: %s",
				c.Request.Method, c.Request.URL.Path, status, elapsed, c.ClientIP())
		} else if status >= 400 {
			logger.Warnf("HTTP %s %s - Status: %d, Duration: %v, IP: %s",
				c.Request.Method, c.Request.URL.Path, status, elapsed, c.ClientIP())
		} else {
			logger.Infof("HTTP %s %s - Status: %d, Duration: %v, IP: %s",
				c.Request.Method, c.Request.URL.Path, status, elapsed, c.ClientIP())
		}
	}
}

// JSONResponse sends a JSON response with the specified HTTP status code.
func JSONResponse(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// ErrorResponse logs and sends a standardized error response with HTTP 400 status.
func ErrorResponse(c *gin.Context, err error) {
	logger.Errorf("API Error: %v", err)
	c.JSON(http.StatusBadRequest, gin.H{
		"error": err.Error(),
	})
}

// Additional utility functions for different log levels
func LogInfo(msg string, args ...interface{}) {
	logger.Infof(msg, args...)
}

// LogDebug logs a formatted debug-level message.
func LogDebug(msg string, args ...interface{}) {
	logger.Debugf(msg, args...)
}

// LogWarn logs a formatted warning-level message.
func LogWarn(msg string, args ...interface{}) {
	logger.Warnf(msg, args...)
}

// LogError logs a formatted error-level message.
func LogError(msg string, args ...interface{}) {
	logger.Errorf(msg, args...)
}

// LogFatal logs a formatted fatal-level message and exits the program.
func LogFatal(msg string, args ...interface{}) {
	logger.Fatalf(msg, args...)
}
