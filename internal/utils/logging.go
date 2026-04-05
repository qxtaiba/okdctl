package utils

import "log/slog"

// Logger is the structured logger used across the codebase. It is an alias
// for *slog.Logger so any handler configured at the slog level (JSON, text,
// level filters) flows through to all call sites without rewrapping.
type Logger = *slog.Logger

// NoopLogger returns a Logger that discards all output. Useful for tests
// and code paths that must not emit logs.
func NoopLogger() Logger {
	return slog.New(slog.DiscardHandler)
}

// DefaultLogger returns the package-level slog.Default() logger.
// Use this when you want global configuration (JSON vs text, level, handlers)
// to apply automatically.
func DefaultLogger() Logger {
	return slog.Default()
}
