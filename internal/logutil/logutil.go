// Package logutil provides small slog.Logger helpers shared across packages.
package logutil

import "log/slog"

// DefaultLogFileName is the per-workspace log file that deploy, destroy,
// and cleanup append to by default (<workspace>/okdctl.log). debug-bundle
// falls back to it when --log-file was not set on the failing run.
const DefaultLogFileName = "okdctl.log"

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

// ProgressReporter starts a progress indicator for desc and returns an
// idempotent stop func; defined here (not tui) to avoid a tui->logutil->tui import cycle.
type ProgressReporter func(desc string) (stop func())

// NopProgressReporter is the no-op ProgressReporter; domain constructors use
// it as the default so callers can invoke the reporter unconditionally.
var NopProgressReporter ProgressReporter = func(string) func() { return func() {} }

// StatusLineReporter starts an updatable status line for desc, returning a
// set func (updates the live detail) and an idempotent stop func — both safe
// to call unconditionally. Same import-cycle reason as ProgressReporter.
type StatusLineReporter func(desc string) (set func(detail string), stop func())

// NopStatusLineReporter is the no-op StatusLineReporter used as the default so
// phases can drive the status line unconditionally.
var NopStatusLineReporter StatusLineReporter = func(string) (func(string), func()) {
	return func(string) {}, func() {}
}
