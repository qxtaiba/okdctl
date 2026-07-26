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

// LF is the short-form constructor for a LogField. Prefer structured
// attrs over fmt.Sprintf so RedactHandler can scrub secret-bearing values.
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
// SimpleLogger. h is wrapped in RedactHandler here — the single choke
// point — so no installed sink can bypass redaction. internal/tui installs
// its styled stderr handler at package init and again from SetRunID; until
// then records fall back to a plain text handler on stderr, so binaries and
// tests that never link tui still log with redaction intact.
func InstallHandler(h slog.Handler) {
	facade.Store(slog.New(NewRedactHandler(h)))
}

// SimpleLogger returns a *slog.Logger backed by the currently installed
// handler (the tui-styled stderr sink in the okdctl binary). stdout stays
// reserved for user-requested data (`okdctl config show`, kubeconfig,
// `releases list --format=json`). Every record passes through RedactHandler
// so credentials in structured attrs never reach the sink.
//
// The returned logger is a snapshot of the current installation. It does
// not auto-update if tui.SetRunID reinstalls the handler afterwards;
// callers that need run_id must be invoked after SetRunID.
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

// Debug emits a debug-level record on stderr. Stdout is reserved for data
// the user explicitly asked for (config show, kubeconfig, JSON output).
// Records pass through RedactHandler via the installed facade logger.
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
