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
	// stdoutLogger backs charmlog Print* output (json/text payloads, kubeconfig
	// contents). It is intentionally NOT routed through slog.Default — the
	// RedactHandler discipline runs only over the stderr sink. Any future
	// slog.Logger that targets stdout must wrap with logutil.NewRedactHandler.
	stdoutLogger atomic.Pointer[charmlog.Logger]
	stderrLogger atomic.Pointer[charmlog.Logger]
)

func init() {
	stdoutLogger.Store(buildLogger(os.Stdout))
	stderrLogger.Store(buildLogger(os.Stderr))
	logutil.InstallHandler(newStderrHandler())
}

// newStderrHandler wraps the current stderrLogger binding for installation
// into logutil; logutil adds the RedactHandler layer on install.
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

// stderrHandler is a slog.Handler that writes every record to stderr,
// coordinating with lineReg so an active spinner/progress line is cleared
// before the record paints. stdoutLogger is retained in the package only so
// ConfigureLoggers has a writer to swap when a caller injects a fake stdout
// in tests.
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

// FormatText and FormatJSON are the two log/output encodings ConfigureLoggers
// accepts. They are the single home for this vocabulary; the cli package's
// --output values alias onto these rather than re-spelling the literals.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// ConfigureLoggers applies level, formatter, and writer settings to the
// package-level loggers. stdoutW and stderrW replace the current outputs;
// level must be a charmlog level string (debug/info/warn/error).
// format must be FormatText or FormatJSON. progressBars controls whether
// logutil.ProgressBarsEnabled returns true. Not safe for concurrent calls —
// call once during cobra PersistentPreRunE before any subcommand runs.
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

// SuppressInfo raises the stderr logger to ErrorLevel, silencing Info and
// Warn records. When --format=json, callers use this so 2>&1 | jq pipelines
// don't see info chatter mixed with the JSON document.
func SuppressInfo() {
	stderrLogger.Load().SetLevel(charmlog.ErrorLevel)
}

// SetRunID pins run_id on the package-level loggers so every subsequent
// logutil.X call carries the correlation ID. Call once at the top of a
// deploy/destroy run — before credential loading, config loading, or
// any log line — so the whole invocation shares a single ID. The
// provisioner's slog.Logger snapshot (logutil.SimpleLogger) captures the
// pinned loggers at createOKDProvisioner time. Not safe for concurrent
// callers.
func SetRunID(id string) {
	logutil.SetRunID(id)
	stdoutLogger.Store(stdoutLogger.Load().With("run_id", id))
	stderrLogger.Store(stderrLogger.Load().With("run_id", id))
	// Reinstall so the facade captures the new stderrLogger value.
	logutil.InstallHandler(newStderrHandler())
	// Rebind slog.SetDefault so third-party libs and background goroutines
	// that captured slog.Default() before SetRunID also observe run_id.
	slog.SetDefault(logutil.SimpleLogger())
}
