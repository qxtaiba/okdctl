// Package logutil provides small slog.Logger helpers shared across packages.
package logutil

import "log/slog"

// NopLogger discards all records after RedactHandler, so a future accidental
// call site still gets its credential-bearing attrs redacted. Use as the
// zero-value fallback for an optional *slog.Logger param.
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
