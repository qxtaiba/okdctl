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
	sinkLogger   atomic.Pointer[charmlog.Logger]
	stderrIsJSON atomic.Bool
)

func init() {
	stderrLogger.Store(buildLogger(os.Stderr))
	logutil.InstallHandler(newStderrHandler())
}

func newStderrHandler() slog.Handler {
	h := &stderrHandler{stderr: stderrLogger.Load()}
	if sl := sinkLogger.Load(); sl != nil {
		h.sink = sl
	}
	return h
}

// badgeWidth keeps every level badge the same width so messages share a column.
const badgeWidth = 7

func levelBadge(c color.Color, text string, bold bool) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Bold(bold).Width(badgeWidth).SetString(text)
}

func buildStyles() *charmlog.Styles {
	styles := charmlog.DefaultStyles()
	// TextFaint resolves to the same hex on both polarities, so a badge built
	// before the wizard flips the theme never reads stale.
	styles.Levels[charmlog.DebugLevel] = levelBadge(ColorTextFaint(), "[DEBUG]", false)
	styles.Levels[charmlog.InfoLevel] = levelBadge(ColorInfo(), "[INFO]", true)
	styles.Levels[charmlog.WarnLevel] = levelBadge(ColorWarning(), "[WARN]", true)
	styles.Levels[charmlog.ErrorLevel] = levelBadge(ColorError(), "[ERROR]", true)
	return styles
}

func buildLogger(w io.Writer) *charmlog.Logger {
	l := charmlog.New(w)
	l.SetReportTimestamp(false)
	l.SetLevel(charmlog.InfoLevel)
	l.SetStyles(buildStyles())
	return l
}

func buildSinkLogger(w io.Writer) *charmlog.Logger {
	l := charmlog.New(w)
	l.SetReportTimestamp(true)
	l.SetFormatter(charmlog.TextFormatter)
	l.SetColorProfile(colorprofile.NoTTY)
	l.SetStyles(buildStyles())
	return l
}

type stderrHandler struct {
	stderr slog.Handler
	sink   slog.Handler // nil when no persistent-log sink is active
}

func (h *stderrHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.stderr.Enabled(ctx, lvl)
}

func (h *stderrHandler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires value receiver
	err := h.stderr.Handle(ctx, r)
	if h.sink == nil {
		return err
	}
	if sinkErr := h.sink.Handle(ctx, r); err == nil {
		err = sinkErr
	}
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

// FormatText and FormatJSON are ConfigureLoggers' output encodings.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// LoggerConfig configures the package-level stderr and sink loggers.
type LoggerConfig struct {
	Level  string
	Format string
	Stderr io.Writer
	// Sink is the persistent run log, always text with timestamps and run_id;
	// nil disables it.
	Sink io.Writer
}

func parseLevelAndFormat(level, format string) (charmlog.Level, charmlog.Formatter, error) {
	lvl, err := charmlog.ParseLevel(level)
	if err != nil {
		return 0, charmlog.TextFormatter, fmt.Errorf("unknown log level %q: %w", level, err)
	}
	var formatter charmlog.Formatter
	switch format {
	case FormatText:
		formatter = charmlog.TextFormatter
	case FormatJSON:
		formatter = charmlog.JSONFormatter
	default:
		return 0, charmlog.TextFormatter, fmt.Errorf("unknown log format %q: must be text or json", format)
	}
	return lvl, formatter, nil
}

// ConfigureLoggers applies cfg to the package-level loggers. Not safe for
// concurrent calls; call once in PersistentPreRunE.
func ConfigureLoggers(cfg LoggerConfig) error {
	lvl, formatter, err := parseLevelAndFormat(cfg.Level, cfg.Format)
	if err != nil {
		return err
	}

	el := stderrLogger.Load()
	el.SetLevel(lvl)
	el.SetFormatter(formatter)
	el.SetOutput(cfg.Stderr)
	if !colorEnabled() {
		// NoTTY, not Ascii: colorprofile.Writer leaves bold/reset codes at Ascii.
		el.SetColorProfile(colorprofile.NoTTY)
	}
	stderrIsJSON.Store(cfg.Format == FormatJSON)
	// execute() pins run_id before --log-format is parsed, so a json run
	// re-attaches it here.
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

	logutil.InstallHandler(newStderrHandler())
	return nil
}

// NewLogHandler builds a redacting handler for w with ConfigureLoggers'
// level and format policy.
func NewLogHandler(level, format string, w io.Writer) (slog.Handler, error) {
	lvl, formatter, err := parseLevelAndFormat(level, format)
	if err != nil {
		return nil, err
	}
	l := buildSinkLogger(w)
	l.SetLevel(lvl)
	l.SetFormatter(formatter)
	if id := logutil.RunID(); id != "" {
		return logutil.NewRedactHandler(l.With("run_id", id)), nil
	}
	return logutil.NewRedactHandler(l), nil
}

// SuppressInfo raises the stderr logger to ErrorLevel so json pipelines see
// no chatter.
func SuppressInfo() {
	stderrLogger.Load().SetLevel(charmlog.ErrorLevel)
}

// SetRunID pins run_id on the package-level loggers, on stderr only under
// json format. Call once before any log line; not safe for concurrent
// callers.
func SetRunID(id string) {
	logutil.SetRunID(id)
	if sl := sinkLogger.Load(); sl != nil {
		sinkLogger.Store(sl.With("run_id", id))
	}
	if stderrIsJSON.Load() {
		stderrLogger.Store(stderrLogger.Load().With("run_id", id))
	}
	logutil.InstallHandler(newStderrHandler())
	// Goroutines that captured slog.Default() earlier must see run_id too.
	slog.SetDefault(logutil.SimpleLogger())
}
