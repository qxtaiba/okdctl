package tui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"

	"github.com/qxtaiba/okdctl/internal/logutil"
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
	// stdoutLogger backs charmlog Print* output (json/text payloads, kubeconfig
	// contents). It is intentionally NOT routed through slog.Default — the
	// RedactHandler discipline runs only over stderrSlog. Any future slog.Logger
	// that targets stdout must wrap with logutil.NewRedactHandler.
	stdoutLogger atomic.Pointer[charmlog.Logger]
	stderrLogger atomic.Pointer[charmlog.Logger]
	// stderrSlog routes Debug/Info/Warn/Error through logutil.RedactHandler
	// so secret-bearing structured attrs (password/token/secret/api_key)
	// are scrubbed before reaching charmlog. SetRunID rebuilds this wrapper
	// whenever stderrLogger is rebound via .With().
	stderrSlog         atomic.Pointer[slog.Logger]
	progressBarsActive atomic.Bool
	runID              atomic.Pointer[string]
)

func init() {
	progressBarsActive.Store(true)
	stdoutLogger.Store(buildLogger(os.Stdout))
	stderrLogger.Store(buildLogger(os.Stderr))
	stderrSlog.Store(buildStderrSlog())
}

func buildStderrSlog() *slog.Logger {
	return slog.New(logutil.NewRedactHandler(&stderrHandler{h: stderrLogger.Load()}))
}

func buildLogger(w io.Writer) *charmlog.Logger {
	l := charmlog.New(w)
	l.SetReportTimestamp(false)
	l.SetLevel(charmlog.InfoLevel)
	styles := charmlog.DefaultStyles()
	styles.Levels[charmlog.DebugLevel] = lipgloss.NewStyle().Foreground(ColorSlate500).SetString("[DEBUG]")
	styles.Levels[charmlog.InfoLevel] = lipgloss.NewStyle().Foreground(ColorInfo).Bold(true).SetString("[INFO]")
	styles.Levels[charmlog.WarnLevel] = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).SetString("[WARN]")
	styles.Levels[charmlog.ErrorLevel] = lipgloss.NewStyle().Foreground(ColorError).Bold(true).SetString("[ERROR]")
	l.SetStyles(styles)
	return l
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
// Records pass through logutil.RedactHandler via stderrSlog.
func Debug(msg string, fields ...LogField) { stderrSlog.Load().Debug(msg, fieldsToArgs(fields)...) }

// Info logs at INFO through the RedactHandler-wrapped stderr slog.
func Info(msg string, fields ...LogField) { stderrSlog.Load().Info(msg, fieldsToArgs(fields)...) }

// Warn logs at WARN through the RedactHandler-wrapped stderr slog.
func Warn(msg string, fields ...LogField) { stderrSlog.Load().Warn(msg, fieldsToArgs(fields)...) }

// Error logs at ERROR through the RedactHandler-wrapped stderr slog.
func Error(msg string, fields ...LogField) { stderrSlog.Load().Error(msg, fieldsToArgs(fields)...) }

// stderrHandler is a slog.Handler that writes every record to stderr.
// stdoutLogger is retained in the package only so ConfigureLoggers has a
// writer to swap when a caller injects a fake stdout in tests.
type stderrHandler struct {
	h slog.Handler
}

func (h *stderrHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.h.Enabled(ctx, lvl)
}

func (h *stderrHandler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	var err error
	lineReg.withLine(func() { err = h.h.Handle(ctx, r) })
	return err
}

func (h *stderrHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &stderrHandler{h: h.h.WithAttrs(attrs)}
}

func (h *stderrHandler) WithGroup(name string) slog.Handler {
	return &stderrHandler{h: h.h.WithGroup(name)}
}

// SimpleLogger returns a *slog.Logger whose records render through the
// styled charm.land/log/v2 formatter on stderr. stdout stays reserved for
// user-requested data (`okdctl config show`, kubeconfig, `releases list
// --format=json`). Every record passes through logutil.RedactHandler so
// credentials in structured attrs never reach the sink.
//
// The returned logger is a snapshot of the current stderrLogger binding.
// It does not auto-update if SetRunID is called after SimpleLogger
// returns. Callers that need run_id must be invoked after SetRunID.
func SimpleLogger() *slog.Logger {
	return buildStderrSlog()
}

// ConfigureLoggers applies level, formatter, and writer settings to the
// package-level loggers. stdoutW and stderrW replace the current outputs;
// level must be a charmlog level string (debug/info/warn/error).
// format must be "text" or "json". progressBars controls whether
// ProgressBarsEnabled returns true. Not safe for concurrent calls — call
// once during cobra PersistentPreRunE before any subcommand runs.
func ConfigureLoggers(level, format string, stdoutW, stderrW io.Writer, progressBars bool) error {
	lvl, err := charmlog.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("unknown log level %q: %w", level, err)
	}

	var formatter charmlog.Formatter
	switch format {
	case "text":
		formatter = charmlog.TextFormatter
	case "json":
		formatter = charmlog.JSONFormatter
	default:
		return fmt.Errorf("unknown log format %q: must be text or json", format)
	}

	sl := stdoutLogger.Load()
	sl.SetLevel(lvl)
	sl.SetFormatter(formatter)
	sl.SetOutput(stdoutW)

	el := stderrLogger.Load()
	el.SetLevel(lvl)
	el.SetFormatter(formatter)
	el.SetOutput(stderrW)

	progressBarsActive.Store(progressBars)
	return nil
}

// ProgressBarsEnabled reports whether progress bars should be rendered.
// False when stdout is not a TTY or when JSON log format is active.
func ProgressBarsEnabled() bool {
	return progressBarsActive.Load()
}

// SuppressInfo raises the stderr logger to ErrorLevel, silencing Info and
// Warn records. When --format=json, callers use this so 2>&1 | jq pipelines
// don't see info chatter mixed with the JSON document.
func SuppressInfo() {
	stderrLogger.Load().SetLevel(charmlog.ErrorLevel)
}

// SetRunID pins run_id on the package-level loggers so every subsequent
// tui.X call carries the correlation ID. Call once at the top of a
// deploy/destroy run — before credential loading, config loading, or
// any log line — so the whole invocation shares a single ID. The
// provisioner's slog.Logger snapshot (SimpleLogger) captures the
// pinned loggers at createOKDProvisioner time. Not safe for concurrent
// callers.
func SetRunID(id string) {
	runID.Store(&id)
	stdoutLogger.Store(stdoutLogger.Load().With("run_id", id))
	stderrLogger.Store(stderrLogger.Load().With("run_id", id))
	// Rebuild the slog wrapper so it captures the new stderrLogger value.
	stderrSlog.Store(buildStderrSlog())
	// Rebind slog.SetDefault so third-party libs and background goroutines
	// that captured slog.Default() before SetRunID also observe run_id.
	slog.SetDefault(stderrSlog.Load())
}

// RunID returns the correlation ID pinned by the most recent SetRunID
// call, or "" before SetRunID is invoked.
func RunID() string {
	if p := runID.Load(); p != nil {
		return *p
	}
	return ""
}
