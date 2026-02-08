package utils

import "context"

type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)

	DebugContext(ctx context.Context, msg string)
	InfoContext(ctx context.Context, msg string)
	WarnContext(ctx context.Context, msg string)
	ErrorContext(ctx context.Context, msg string)
}

type noopLogger struct{}

func (noopLogger) Debug(string)                         {}
func (noopLogger) Info(string)                          {}
func (noopLogger) Warn(string)                          {}
func (noopLogger) Error(string)                         {}
func (noopLogger) DebugContext(context.Context, string) {}
func (noopLogger) InfoContext(context.Context, string)  {}
func (noopLogger) WarnContext(context.Context, string)  {}
func (noopLogger) ErrorContext(context.Context, string) {}

func NoopLogger() Logger {
	return noopLogger{}
}
