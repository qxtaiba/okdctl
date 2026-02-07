// Package logging provides a unified logging interface for the application.
package logging

import "context"

// Logger defines the standard logging interface used throughout the application.
// It provides both basic logging methods and context-aware variants for request
// tracing and cancellation awareness.
type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)

	// Context-aware logging methods for request tracing and cancellation awareness.
	// These methods allow correlating log entries with request lifecycle and
	// respecting context cancellation.
	DebugContext(ctx context.Context, msg string)
	InfoContext(ctx context.Context, msg string)
	WarnContext(ctx context.Context, msg string)
	ErrorContext(ctx context.Context, msg string)
}

// noopLogger discards all output.
type noopLogger struct{}

func (noopLogger) Debug(string)                         {}
func (noopLogger) Info(string)                          {}
func (noopLogger) Warn(string)                          {}
func (noopLogger) Error(string)                         {}
func (noopLogger) DebugContext(context.Context, string) {}
func (noopLogger) InfoContext(context.Context, string)  {}
func (noopLogger) WarnContext(context.Context, string)  {}
func (noopLogger) ErrorContext(context.Context, string) {}

// NoopLogger returns a logger that discards all output.
func NoopLogger() Logger {
	return noopLogger{}
}
