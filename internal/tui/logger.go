package tui

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"log/slog"
	"os"
	"sync/atomic"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

var (
	stderrLogger atomic.Pointer[charmlog.Logger]
	// sinkLogger is the persistent-run-log logger; nil when no sink is
	// configured.
	sinkLogger atomic.Pointer[charmlog.Logger]
	// stderrIsJSON gates whether SetRunID attaches run_id to stderr;
	// ConfigureLoggers sets it from LoggerConfig.Format.
	stderrIsJSON atomic.Bool
)

func init() {
	stderrLogger.Store(buildLogger(os.Stderr))
	logutil.InstallHandler(newStderrHandler())
}

// newStderrHandler wraps stderrLogger (and sinkLogger, when configured) for
// logutil, which adds the RedactHandler layer on install.
func newStderrHandler() slog.Handler {
	h := &stderrHandler{stderr: stderrLogger.Load()}
	if sl := sinkLogger.Load(); sl != nil {
		h.sink = sl
	}
	return h
}

// badgeWidth pads every level badge to the same column so log messages
// share a starting column regardless of level.
const badgeWidth = 7

func levelBadge(c color.Color, text string, bold bool) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Bold(bold).Width(badgeWidth).SetString(text)
}

func buildStyles() *charmlog.Styles {
	styles := charmlog.DefaultStyles()
	// The DEBUG badge captures ColorSlate500 here, at logger-configuration
	// time — before the wizard's BackgroundColorMsg can call
	// SetDarkBackground — but that's never visibly stale: SetDarkBackground
	// assigns ColorSlate500 the same hex on both the dark and light branch
	// (colors.go), so a badge built pre-wizard is byte-identical to one
	// rebuilt post-wizard. No rebuild hook needed.
	styles.Levels[charmlog.DebugLevel] = levelBadge(ColorSlate500, "[DEBUG]", false)
	styles.Levels[charmlog.InfoLevel] = levelBadge(ColorInfo, "[INFO]", true)
	styles.Levels[charmlog.WarnLevel] = levelBadge(ColorWarning, "[WARN]", true)
	styles.Levels[charmlog.ErrorLevel] = levelBadge(ColorError, "[ERROR]", true)
	return styles
}

func buildLogger(w io.Writer) *charmlog.Logger {
	l := charmlog.New(w)
	l.SetReportTimestamp(false)
	l.SetLevel(charmlog.InfoLevel)
	l.SetStyles(buildStyles())
	return l
}

// buildSinkLogger builds the persistent run-log logger: always text with
// timestamps on and a colour-stripped profile, independent of the stderr
// logger's format and colour settings.
func buildSinkLogger(w io.Writer) *charmlog.Logger {
	l := charmlog.New(w)
	l.SetReportTimestamp(true)
	l.SetFormatter(charmlog.TextFormatter)
	l.SetColorProfile(colorprofile.NoTTY)
	l.SetStyles(buildStyles())
	return l
}

// stderrHandler fans each record to stderr and, when configured, the
// persistent-log sink, clearing any active spinner/progress line via lineReg
// first so both writes land on a clean line.
type stderrHandler struct {
	stderr slog.Handler
	sink   slog.Handler // nil when no persistent-log sink is active
}

func (h *stderrHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.stderr.Enabled(ctx, lvl)
}

func (h *stderrHandler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	var err error
	lineReg.withLine(func() {
		err = h.stderr.Handle(ctx, r)
		if h.sink == nil {
			return
		}
		if sinkErr := h.sink.Handle(ctx, r); err == nil {
			err = sinkErr
		}
	})
	return err
}

func (h *stderrHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := &stderrHandler{stderr: h.stderr.WithAttrs(attrs)}
	if h.sink != nil {
		nh.sink = h.sink.WithAttrs(attrs)
	}
	return nh
}

func (h *stderrHandler) WithGroup(name string) slog.Handler {
	nh := &stderrHandler{stderr: h.stderr.WithGroup(name)}
	if h.sink != nil {
		nh.sink = h.sink.WithGroup(name)
	}
	return nh
}

// FormatText and FormatJSON are ConfigureLoggers' two output encodings.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// LoggerConfig configures the package-level stderr and sink loggers.
type LoggerConfig struct {
	Level  string
	Format string
	Stderr io.Writer
	// Sink is the persistent run log: always text with timestamps on and a
	// colour-stripped profile, and always carries run_id; nil disables it.
	Sink         io.Writer
	ProgressBars bool
}

// ConfigureLoggers applies level, formatter, and writer settings to the
// package-level loggers. Not safe for concurrent calls — call once in cobra
// PersistentPreRunE before any subcommand runs.
func ConfigureLoggers(cfg LoggerConfig) error {
	lvl, err := charmlog.ParseLevel(cfg.Level)
	if err != nil {
		return fmt.Errorf("unknown log level %q: %w", cfg.Level, err)
	}

	var formatter charmlog.Formatter
	switch cfg.Format {
	case FormatText:
		formatter = charmlog.TextFormatter
	case FormatJSON:
		formatter = charmlog.JSONFormatter
	default:
		return fmt.Errorf("unknown log format %q: must be text or json", cfg.Format)
	}

	el := stderrLogger.Load()
	el.SetLevel(lvl)
	el.SetFormatter(formatter)
	el.SetOutput(cfg.Stderr)
	if !colorEnabled() {
		// NoTTY, not Ascii: colorprofile.Writer only downsamples color SGR
		// params at Ascii, leaving bold/reset codes in place (see detect's
		// doc in colorprofile.go); NoTTY takes the full ansi.Strip path so
		// --no-color/NO_COLOR text output carries zero escape bytes.
		el.SetColorProfile(colorprofile.NoTTY)
	}
	stderrIsJSON.Store(cfg.Format == FormatJSON)
	// SetRunID may already have fired (execute() pins run_id before
	// PersistentPreRunE parses --log-format); attach it here too so a json
	// run configured after SetRunID still carries it.
	if cfg.Format == FormatJSON {
		if id := logutil.RunID(); id != "" {
			stderrLogger.Store(el.With("run_id", id))
		}
	}

	if cfg.Sink != nil {
		sl := buildSinkLogger(cfg.Sink)
		sl.SetLevel(lvl)
		if id := logutil.RunID(); id != "" {
			sl = sl.With("run_id", id)
		}
		sinkLogger.Store(sl)
	} else {
		sinkLogger.Store(nil)
	}

	logutil.SetProgressBarsEnabled(cfg.ProgressBars)
	logutil.InstallHandler(newStderrHandler())
	return nil
}

// SuppressInfo raises the stderr logger to ErrorLevel (silencing Info/Warn)
// so --format=json | jq pipelines don't see chatter mixed into the JSON.
func SuppressInfo() {
	stderrLogger.Load().SetLevel(charmlog.ErrorLevel)
}

// SetRunID pins run_id on the package-level loggers so subsequent
// logutil.X calls carry it; call once, before any log line, since
// logutil.SimpleLogger snapshots loggers at creation time. run_id always
// attaches to the sink and attaches to stderr only under json format. Not
// safe for concurrent callers.
func SetRunID(id string) {
	logutil.SetRunID(id)
	if sl := sinkLogger.Load(); sl != nil {
		sinkLogger.Store(sl.With("run_id", id))
	}
	if stderrIsJSON.Load() {
		stderrLogger.Store(stderrLogger.Load().With("run_id", id))
	}
	// Reinstall so the facade captures the rebound loggers.
	logutil.InstallHandler(newStderrHandler())
	// Rebind slog.SetDefault so libs/goroutines that captured slog.Default()
	// earlier also see run_id.
	slog.SetDefault(logutil.SimpleLogger())
}
