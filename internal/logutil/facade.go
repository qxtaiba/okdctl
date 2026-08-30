package logutil

import (
	"log/slog"
	"os"
	"sync/atomic"
)

// LogField is a single structured key/value pair attached to a log record.
type LogField struct {
	Key   string
	Value any
}

// LF is the short-form constructor for a LogField; prefer structured attrs over
// fmt.Sprintf so RedactHandler can scrub secrets.
func LF(key string, value any) LogField {
	return LogField{Key: key, Value: value}
}

var (
	facade             atomic.Pointer[slog.Logger]
	progressBarsActive atomic.Bool
	runID              atomic.Pointer[string]
)

func init() {
	progressBarsActive.Store(true)
	InstallHandler(slog.NewTextHandler(os.Stderr, nil))
}

// InstallHandler makes h the sink behind the package facade and
// SimpleLogger, wrapped in RedactHandler as the single choke point so no
// installed sink can bypass redaction. Binaries that never link
// internal/tui still log through a plain-text fallback with redaction intact.
func InstallHandler(h slog.Handler) {
	facade.Store(slog.New(NewRedactHandler(h)))
}

// SimpleLogger returns a *slog.Logger backed by the currently installed
// handler, snapshotted at call time — it does NOT auto-update if
// tui.SetRunID reinstalls the handler afterward, so callers needing run_id
// must call this after SetRunID.
func SimpleLogger() *slog.Logger {
	return facade.Load()
}

func fieldsToArgs(fields []LogField) []any {
	args := make([]any, 0, len(fields)*2)
	for _, f := range fields {
		args = append(args, f.Key, f.Value)
	}
	return args
}

// Debug logs at DEBUG through the RedactHandler-wrapped facade logger on
// stderr; stdout is reserved for data the user explicitly asked for.
func Debug(msg string, fields ...LogField) { facade.Load().Debug(msg, fieldsToArgs(fields)...) }

// Info logs at INFO through the RedactHandler-wrapped facade logger.
func Info(msg string, fields ...LogField) { facade.Load().Info(msg, fieldsToArgs(fields)...) }

// Warn logs at WARN through the RedactHandler-wrapped facade logger.
func Warn(msg string, fields ...LogField) { facade.Load().Warn(msg, fieldsToArgs(fields)...) }

// Error logs at ERROR through the RedactHandler-wrapped facade logger.
func Error(msg string, fields ...LogField) { facade.Load().Error(msg, fieldsToArgs(fields)...) }

// ProgressBarsEnabled reports whether progress bars should be rendered.
// False when stdout is not a TTY or when JSON log format is active;
// tui.ConfigureLoggers owns the value.
func ProgressBarsEnabled() bool {
	return progressBarsActive.Load()
}

// SetProgressBarsEnabled records the progress-bar gate. Production caller:
// tui.ConfigureLoggers, once during cobra PersistentPreRunE.
func SetProgressBarsEnabled(enabled bool) {
	progressBarsActive.Store(enabled)
}

// SetRunID records the correlation ID returned by RunID. It does not touch
// any sink; tui.SetRunID is the production entry point, which also rebinds
// the styled loggers with the run_id attr and reinstalls the handler.
func SetRunID(id string) {
	runID.Store(&id)
}

// RunID returns the correlation ID pinned by the most recent SetRunID
// call, or "" before SetRunID is invoked.
func RunID() string {
	if p := runID.Load(); p != nil {
		return *p
	}
	return ""
}
