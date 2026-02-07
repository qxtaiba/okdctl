package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

// LogLevel represents log severity.
type LogLevel int

const (
	// LogLevelDebug is for detailed debugging information.
	LogLevelDebug LogLevel = iota
	// LogLevelInfo is for general information.
	LogLevelInfo
	// LogLevelWarn is for warnings.
	LogLevelWarn
	// LogLevelError is for errors.
	LogLevelError
)

// LogField represents a key-value pair for structured logging.
type LogField struct {
	Key   string
	Value interface{}
}

// LF creates a LogField (short for Log Field).
func LF(key string, value interface{}) LogField {
	return LogField{Key: key, Value: value}
}

// Logger provides structured logging with TUI styling.
type Logger struct {
	mu     sync.RWMutex
	level  LogLevel
	fields []LogField
}

// NewLogger creates a new TUI logger.
func NewLogger() *Logger {
	return &Logger{
		level: LogLevelInfo,
	}
}

// Debug logs a debug message.
func (log *Logger) Debug(msg string, fields ...LogField) {
	log.mu.RLock()
	lvl := log.level
	log.mu.RUnlock()
	if lvl > LogLevelDebug {
		return
	}
	fmt.Println(LogDebug(log.format(msg, fields)))
}

// Info logs an info message.
func (log *Logger) Info(msg string, fields ...LogField) {
	log.mu.RLock()
	lvl := log.level
	log.mu.RUnlock()
	if lvl > LogLevelInfo {
		return
	}
	fmt.Println(LogInfo(log.format(msg, fields)))
}

// Warn logs a warning message.
func (log *Logger) Warn(msg string, fields ...LogField) {
	log.mu.RLock()
	lvl := log.level
	log.mu.RUnlock()
	if lvl > LogLevelWarn {
		return
	}
	fmt.Println(LogWarn(log.format(msg, fields)))
}

// Error logs an error message. Errors are always logged regardless of log level.
func (log *Logger) Error(msg string, fields ...LogField) {
	fmt.Println(LogError(log.format(msg, fields)))
}

// format formats the message with any fields.
func (log *Logger) format(msg string, fields []LogField) string {
	log.mu.RLock()
	logFields := make([]LogField, len(log.fields))
	copy(logFields, log.fields)
	log.mu.RUnlock()

	allFields := make([]LogField, 0, len(logFields)+len(fields))
	allFields = append(allFields, logFields...)
	allFields = append(allFields, fields...)
	if len(allFields) == 0 {
		return msg
	}

	var parts []string
	for _, f := range allFields {
		parts = append(parts, fmt.Sprintf("%s=%v", f.Key, f.Value))
	}

	return fmt.Sprintf("%s [%s]", msg, strings.Join(parts, " "))
}

var defaultLogger = NewLogger()

// Debug logs a debug message using the default logger.
func Debug(msg string, fields ...LogField) {
	defaultLogger.Debug(msg, fields...)
}

// Info logs an info message using the default logger.
func Info(msg string, fields ...LogField) {
	defaultLogger.Info(msg, fields...)
}

// Warn logs a warning message using the default logger.
func Warn(msg string, fields ...LogField) {
	defaultLogger.Warn(msg, fields...)
}

// Error logs an error message using the default logger.
func Error(msg string, fields ...LogField) {
	defaultLogger.Error(msg, fields...)
}

// simpleLogger adapts the TUI logger to logging.Logger.
type simpleLogger struct{}

func (simpleLogger) Debug(msg string) { defaultLogger.Debug(msg) }
func (simpleLogger) Info(msg string)  { defaultLogger.Info(msg) }
func (simpleLogger) Warn(msg string)  { defaultLogger.Warn(msg) }
func (simpleLogger) Error(msg string) { defaultLogger.Error(msg) }

// Context-aware methods check if context is done before logging.
func (s simpleLogger) DebugContext(ctx context.Context, msg string) {
	if ctx.Err() == nil {
		s.Debug(msg)
	}
}
func (s simpleLogger) InfoContext(ctx context.Context, msg string) {
	if ctx.Err() == nil {
		s.Info(msg)
	}
}
func (s simpleLogger) WarnContext(ctx context.Context, msg string) {
	if ctx.Err() == nil {
		s.Warn(msg)
	}
}
func (s simpleLogger) ErrorContext(ctx context.Context, msg string) {
	if ctx.Err() == nil {
		s.Error(msg)
	}
}

// SimpleLogger returns a logging.Logger that writes to the TUI.
func SimpleLogger() logging.Logger {
	return simpleLogger{}
}
