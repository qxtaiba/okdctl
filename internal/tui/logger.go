package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
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
	_, _ = fmt.Fprintln(os.Stdout, logDebug(log.format(msg, fields)))
}

func (log *Logger) Info(msg string, fields ...LogField) {
	log.mu.RLock()
	lvl := log.level
	log.mu.RUnlock()
	if lvl > LogLevelInfo {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, LogInfo(log.format(msg, fields)))
}

func (log *Logger) Warn(msg string, fields ...LogField) {
	log.mu.RLock()
	lvl := log.level
	log.mu.RUnlock()
	if lvl > LogLevelWarn {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, LogWarn(log.format(msg, fields)))
}

func (log *Logger) Error(msg string, fields ...LogField) {
	fmt.Fprintln(os.Stderr, LogError(log.format(msg, fields)))
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

// simpleHandler is a slog.Handler that routes records through the tui
// default logger so structured-log call sites in the wider codebase print
// with the same styled output as direct tui.Info/tui.Warn/... calls.
type simpleHandler struct {
	attrs  []slog.Attr
	groups []string
}

func (h *simpleHandler) Enabled(_ context.Context, level slog.Level) bool {
	defaultLogger.mu.RLock()
	lvl := defaultLogger.level
	defaultLogger.mu.RUnlock()
	return slogLevelToTUI(level) >= lvl
}

func (h *simpleHandler) Handle(_ context.Context, r slog.Record) error {
	var prefix string
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}
	fields := make([]LogField, 0, len(h.attrs)+r.NumAttrs())
	for _, a := range h.attrs {
		fields = append(fields, LogField{Key: prefix + a.Key, Value: a.Value.Any()})
	}
	r.Attrs(func(a slog.Attr) bool {
		fields = append(fields, LogField{Key: prefix + a.Key, Value: a.Value.Any()})
		return true
	})
	switch {
	case r.Level >= slog.LevelError:
		defaultLogger.Error(r.Message, fields...)
	case r.Level >= slog.LevelWarn:
		defaultLogger.Warn(r.Message, fields...)
	case r.Level >= slog.LevelInfo:
		defaultLogger.Info(r.Message, fields...)
	default:
		defaultLogger.Debug(r.Message, fields...)
	}
	return nil
}

func (h *simpleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &simpleHandler{attrs: merged, groups: h.groups}
}

func (h *simpleHandler) WithGroup(name string) slog.Handler {
	groups := make([]string, 0, len(h.groups)+1)
	groups = append(groups, h.groups...)
	groups = append(groups, name)
	return &simpleHandler{attrs: h.attrs, groups: groups}
}

func slogLevelToTUI(l slog.Level) LogLevel {
	switch {
	case l >= slog.LevelError:
		return LogLevelError
	case l >= slog.LevelWarn:
		return LogLevelWarn
	case l >= slog.LevelInfo:
		return LogLevelInfo
	default:
		return LogLevelDebug
	}
}

// SimpleLogger returns a *slog.Logger (alias for *slog.Logger) whose records
// are printed through the tui styled formatters. Use this to wire CLI-facing
// log output into subsystems that expect a structured logger.
func SimpleLogger() *slog.Logger {
	return slog.New(&simpleHandler{})
}
