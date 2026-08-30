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

var (
	// stdoutLogger is stderrLogger's stdout counterpart; unused today.
	// Future writers must wrap with logutil.NewRedactHandler — RedactHandler
	// only wraps the stderr sink.
	stdoutLogger atomic.Pointer[charmlog.Logger]
	stderrLogger atomic.Pointer[charmlog.Logger]
)

func init() {
	stdoutLogger.Store(buildLogger(os.Stdout))
	stderrLogger.Store(buildLogger(os.Stderr))
	logutil.InstallHandler(newStderrHandler())
}

// newStderrHandler wraps stderrLogger for logutil, which adds the
// RedactHandler layer on install.
func newStderrHandler() slog.Handler {
	return &stderrHandler{h: stderrLogger.Load()}
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

// stderrHandler writes every record to stderr, clearing any active
// spinner/progress line via lineReg first.
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

// FormatText and FormatJSON are ConfigureLoggers' two output encodings.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// ConfigureLoggers applies level, formatter, and writer settings to the
// package-level loggers. Not safe for concurrent calls — call once in cobra
// PersistentPreRunE before any subcommand runs.
func ConfigureLoggers(level, format string, stdoutW, stderrW io.Writer, progressBars bool) error {
	lvl, err := charmlog.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("unknown log level %q: %w", level, err)
	}

	var formatter charmlog.Formatter
	switch format {
	case FormatText:
		formatter = charmlog.TextFormatter
	case FormatJSON:
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

	logutil.SetProgressBarsEnabled(progressBars)
	return nil
}

// SuppressInfo raises the stderr logger to ErrorLevel (silencing Info/Warn)
// so --format=json | jq pipelines don't see chatter mixed into the JSON.
func SuppressInfo() {
	stderrLogger.Load().SetLevel(charmlog.ErrorLevel)
}

// SetRunID pins run_id on the package-level loggers so subsequent
// logutil.X calls carry it; call once, before any log line, since
// logutil.SimpleLogger snapshots loggers at creation time. Not safe for
// concurrent callers.
func SetRunID(id string) {
	logutil.SetRunID(id)
	stdoutLogger.Store(stdoutLogger.Load().With("run_id", id))
	stderrLogger.Store(stderrLogger.Load().With("run_id", id))
	// Reinstall so the facade captures the new stderrLogger.
	logutil.InstallHandler(newStderrHandler())
	// Rebind slog.SetDefault so libs/goroutines that captured slog.Default()
	// earlier also see run_id.
	slog.SetDefault(logutil.SimpleLogger())
}
