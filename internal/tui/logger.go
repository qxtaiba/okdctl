package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

type LogField struct {
	Key   string
	Value interface{}
}

func LF(key string, value interface{}) LogField {
	return LogField{Key: key, Value: value}
}

type Logger struct {
	mu     sync.RWMutex
	level  LogLevel
	fields []LogField
}

func NewLogger() *Logger {
	return &Logger{
		level: LogLevelInfo,
	}
}

func (log *Logger) Debug(msg string, fields ...LogField) {
	log.mu.RLock()
	lvl := log.level
	log.mu.RUnlock()
	if lvl > LogLevelDebug {
		return
	}
	fmt.Println(LogDebug(log.format(msg, fields)))
}

func (log *Logger) Info(msg string, fields ...LogField) {
	log.mu.RLock()
	lvl := log.level
	log.mu.RUnlock()
	if lvl > LogLevelInfo {
		return
	}
	fmt.Println(LogInfo(log.format(msg, fields)))
}

func (log *Logger) Warn(msg string, fields ...LogField) {
	log.mu.RLock()
	lvl := log.level
	log.mu.RUnlock()
	if lvl > LogLevelWarn {
		return
	}
	fmt.Println(LogWarn(log.format(msg, fields)))
}

func (log *Logger) Error(msg string, fields ...LogField) {
	fmt.Println(LogError(log.format(msg, fields)))
}

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

func Debug(msg string, fields ...LogField) {
	defaultLogger.Debug(msg, fields...)
}

func Info(msg string, fields ...LogField) {
	defaultLogger.Info(msg, fields...)
}

func Warn(msg string, fields ...LogField) {
	defaultLogger.Warn(msg, fields...)
}

func Error(msg string, fields ...LogField) {
	defaultLogger.Error(msg, fields...)
}

type simpleLogger struct{}

func (simpleLogger) Debug(msg string) { defaultLogger.Debug(msg) }
func (simpleLogger) Info(msg string)  { defaultLogger.Info(msg) }
func (simpleLogger) Warn(msg string)  { defaultLogger.Warn(msg) }
func (simpleLogger) Error(msg string) { defaultLogger.Error(msg) }

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

func SimpleLogger() utils.Logger {
	return simpleLogger{}
}
