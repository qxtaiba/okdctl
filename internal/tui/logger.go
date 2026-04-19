package tui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// LogLevel enumerates the severity thresholds accepted by the TUI logger.
type LogLevel int

// LogLevel values ordered from most to least verbose.
const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// LogField is a single structured key/value pair attached to a log record.
type LogField struct {
	Key   string
	Value any
}

// LF is a shorthand constructor for LogField, producing concise call sites
// for the Debug/Info/Warn/Error helpers.
func LF(key string, value any) LogField {
	return LogField{Key: key, Value: value}
}

var (
	stdoutLogger       = buildLogger(os.Stdout)
	stderrLogger       = buildLogger(os.Stderr)
	progressBarsActive = true
)

func buildLogger(w io.Writer) *charmlog.Logger {
	l := charmlog.New(w)
	l.SetReportTimestamp(false)
	l.SetLevel(charmlog.InfoLevel)
	styles := charmlog.DefaultStyles()
	styles.Levels[charmlog.DebugLevel] = lipgloss.NewStyle().Foreground(ColorSlate500).SetString("[DEBUG]")
	styles.Levels[charmlog.InfoLevel] = lipgloss.NewStyle().Foreground(ColorCyan500).Bold(true).SetString("[INFO]")
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
func Debug(msg string, fields ...LogField) { stderrLogger.Debug(msg, fieldsToArgs(fields)...) }

// Info emits an info-level record on stderr.
func Info(msg string, fields ...LogField) { stderrLogger.Info(msg, fieldsToArgs(fields)...) }

// Warn emits a warn-level record on stderr.
func Warn(msg string, fields ...LogField) { stderrLogger.Warn(msg, fieldsToArgs(fields)...) }

// Error emits an error-level record on stderr.
func Error(msg string, fields ...LogField) { stderrLogger.Error(msg, fieldsToArgs(fields)...) }

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
	return h.h.Handle(ctx, r)
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
func SimpleLogger() *slog.Logger {
	return slog.New(logutil.NewRedactHandler(&stderrHandler{h: stderrLogger}))
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

	stdoutLogger.SetLevel(lvl)
	stdoutLogger.SetFormatter(formatter)
	stdoutLogger.SetOutput(stdoutW)

	stderrLogger.SetLevel(lvl)
	stderrLogger.SetFormatter(formatter)
	stderrLogger.SetOutput(stderrW)

	progressBarsActive = progressBars
	return nil
}

// ProgressBarsEnabled reports whether progress bars should be rendered.
// False when stdout is not a TTY or when JSON log format is active.
func ProgressBarsEnabled() bool {
	return progressBarsActive
}
