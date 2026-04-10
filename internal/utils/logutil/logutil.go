// Package logutil provides small slog.Logger helpers shared across packages.
package logutil

import "log/slog"

// NopLogger discards all log records. Use it as a zero-value fallback for
// constructors that accept an optional *slog.Logger, or for tests that don't
// care about log output.
var NopLogger = slog.New(slog.DiscardHandler)
