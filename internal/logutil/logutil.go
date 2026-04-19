// Package logutil provides small slog.Logger helpers shared across packages.
package logutil

import "log/slog"

// NopLogger discards all log records. Use it as a zero-value fallback for
// constructors that accept an optional *slog.Logger, or for tests that don't
// care about log output.
var NopLogger = slog.New(slog.DiscardHandler)

// OrNop returns l when non-nil, otherwise NopLogger. Use at the top of any
// function taking an optional *slog.Logger so the body can log unconditionally
// without per-site nil guards.
func OrNop(l *slog.Logger) *slog.Logger {
	if l == nil {
		return NopLogger
	}
	return l
}
