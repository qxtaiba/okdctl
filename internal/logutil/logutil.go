// Package logutil provides small slog.Logger helpers shared across packages.
package logutil

import "log/slog"

// NopLogger discards all log records after passing them through
// RedactHandler. Use it as a zero-value fallback for constructors that
// accept an optional *slog.Logger, or for tests that don't care about log
// output. The RedactHandler layer ensures any credential-bearing attr is
// redacted even if a future call site logs to NopLogger inadvertently.
var NopLogger = slog.New(NewRedactHandler(slog.DiscardHandler))

// OrNop returns l when non-nil, otherwise NopLogger. Use at the top of any
// function taking an optional *slog.Logger so the body can log unconditionally
// without per-site nil guards.
func OrNop(l *slog.Logger) *slog.Logger {
	if l == nil {
		return NopLogger
	}
	return l
}

// ProgressReporter starts a progress indicator for desc and returns a stop
// func. The stop func MUST be idempotent. Implementations may discard desc.
type ProgressReporter func(desc string) (stop func())

// NopProgressReporter is the no-op ProgressReporter; domain constructors use
// it as the default so callers can invoke the reporter unconditionally.
var NopProgressReporter ProgressReporter = func(string) func() { return func() {} }
