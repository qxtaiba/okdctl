package tui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
)

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

type LogField struct {
	Key   string
	Value any
}

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

func Debug(msg string, fields ...LogField) { stdoutLogger.Debug(msg, fieldsToArgs(fields)...) }
func Info(msg string, fields ...LogField)  { stdoutLogger.Info(msg, fieldsToArgs(fields)...) }
func Warn(msg string, fields ...LogField)  { stdoutLogger.Warn(msg, fieldsToArgs(fields)...) }
func Error(msg string, fields ...LogField) { stderrLogger.Error(msg, fieldsToArgs(fields)...) }

// dualHandler splits slog records by level so Error records go to stderr
// while Info/Warn/Debug go to stdout — matching the direct tui.Error vs
// tui.Info/Warn/Debug stream split on the package-level helpers above.
type dualHandler struct {
	stdout, stderr slog.Handler
}

func (h *dualHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.stdout.Enabled(ctx, lvl)
}

func (h *dualHandler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	if r.Level >= slog.LevelError {
		return h.stderr.Handle(ctx, r)
	}
	return h.stdout.Handle(ctx, r)
}

func (h *dualHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &dualHandler{stdout: h.stdout.WithAttrs(attrs), stderr: h.stderr.WithAttrs(attrs)}
}

func (h *dualHandler) WithGroup(name string) slog.Handler {
	return &dualHandler{stdout: h.stdout.WithGroup(name), stderr: h.stderr.WithGroup(name)}
}

// SimpleLogger returns a *slog.Logger whose records render through the
// styled charm.land/log/v2 formatter. Error records route to stderr,
// everything else to stdout.
func SimpleLogger() *slog.Logger {
	return slog.New(&dualHandler{stdout: stdoutLogger, stderr: stderrLogger})
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
