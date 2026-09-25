package deployexec

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// flowStepCount is how many screens NewSteps assembles, the step count the
// frame's split gate is evaluated against; TestFlowStepCountMatchesNewSteps
// pins it.
const flowStepCount = 2

// narrowTailRows is how many log lines ride under the checklist when the frame
// is too narrow to give the log a pane of its own.
const narrowTailRows = 6

// logStampFormat is the wall-clock stamp each log row leads with; seconds
// resolution is as fine as a step-by-step install needs.
const logStampFormat = "15:04:05"

// keyLogLock and keyLogFull are the log viewport's two keys: freeze the window
// where it stands, and swap the log full-screen.
const (
	keyLogLock = 'l'
	keyLogFull = 'f'
)

// logView is a log viewport's own state: where the window ends while follow is
// locked, and whether the log has taken the whole frame.
type logView struct {
	locked bool
	// lockAt is the absolute stream index the locked window ends at (exclusive).
	lockAt int64
	full   bool
}

// frameSize records the terminal a step is laid out against plus the body box
// the frame gives it — neither of which View's own arguments report, since the
// frame calls View with a fixed 1000-row budget and a width its caps have
// already flattened.
type frameSize struct {
	termWidth, termHeight int
	bodyHeight            int
}

// SetTerminalSize records the terminal's own dimensions.
func (f *frameSize) SetTerminalSize(width, height int) {
	f.termWidth, f.termHeight = width, height
}

// splitsFrame reports whether this terminal gives the flow a right-hand pane.
func (f *frameSize) splitsFrame() bool {
	return wizard.SplitsFrame(f.termWidth, f.termHeight, flowStepCount)
}

// logWindow picks the rows a viewport shows from a ring snapshot: the tail while
// following, or the budget of lines ending at view.lockAt while locked, clamped
// to whatever the ring still holds.
func logWindow(lines []LogLine, first int64, view logView, budget int) []LogLine {
	if budget <= 0 || len(lines) == 0 {
		return nil
	}
	end := len(lines)
	if view.locked {
		end = min(max(int(view.lockAt-first), 0), len(lines))
	}
	// A lock older than everything the ring still holds shows the oldest rows it
	// has rather than nothing at all.
	if end == 0 {
		end = min(budget, len(lines))
	}
	return lines[max(end-budget, 0):end]
}

// logRows renders lines as dim, stamped rows within width columns, never
// exceeding budget rows — the oldest are dropped first. A side pane and a narrow
// tail give one row per line and clip the overflow, since a wrapped line there
// costs a second row to show a few trailing fields; the full-screen log wraps
// instead, where there is room to read them.
func logRows(lines []LogLine, width, budget int, wrap bool) []string {
	if width <= 0 || budget <= 0 {
		return nil
	}
	stampStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate600)
	stampWidth := lipgloss.Width(logStampFormat)
	textWidth := max(width-stampWidth-1, 8)

	var rows []string
	for i := range lines {
		l := &lines[i]
		stamp := stampStyle.Render(l.At.Format(logStampFormat))
		textStyle := logLevelStyle(l.Level)
		text := l.Text
		if tag := logLevelTag(l.Level); tag != "" {
			text = tag + " " + text
		}
		if !wrap {
			rows = append(rows, stamp+" "+textStyle.Render(tui.Truncate(text, textWidth)))
			continue
		}
		wrapped := strings.Split(lipgloss.Wrap(text, textWidth, ""), "\n")
		for j, part := range wrapped {
			if j == 0 {
				rows = append(rows, stamp+" "+textStyle.Render(part))
				continue
			}
			rows = append(rows, strings.Repeat(" ", stampWidth+1)+textStyle.Render(part))
		}
	}
	// Wrapping can turn one line into several, so the drop happens after
	// rendering: the newest rows are the ones worth keeping.
	return rows[max(len(rows)-budget, 0):]
}

// logLevelTag returns the textual severity a rendered row leads with — WARN
// and ERROR only, so severity survives NO_COLOR without tagging the whole
// dim info stream.
func logLevelTag(level string) string {
	switch upper := strings.ToUpper(level); upper {
	case "ERROR", "WARN":
		return upper
	default:
		return ""
	}
}

// logLevelStyle returns the colour a captured line renders in: a warning and a
// failure must stand out of the dim stream, since the failure screen's evidence
// is whatever the tail was carrying when the run stopped.
func logLevelStyle(level string) lipgloss.Style {
	switch strings.ToUpper(level) {
	case "ERROR":
		return lipgloss.NewStyle().Foreground(tui.ColorError)
	case "WARN":
		return lipgloss.NewStyle().Foreground(tui.ColorWarning)
	default:
		return lipgloss.NewStyle().Foreground(tui.ColorSlate400)
	}
}

// renderLogPane renders a log viewport into at most height rows of width
// columns: a dim header naming the follow state, then the window's stamped rows,
// tail last. wrap is the full-screen view's line handling; see logRows.
func renderLogPane(src LogSource, view logView, width, height int, wrap bool) string {
	if src == nil || height <= 0 {
		return ""
	}
	header := logPaneHeader(view, width)
	lines, first := src.Snapshot()
	rows := logRows(logWindow(lines, first, view, height-1), width, height-1, wrap)
	if len(rows) == 0 {
		rows = []string{lipgloss.NewStyle().Foreground(tui.ColorSlate600).Render("waiting for the first log line…")}
	}
	return strings.Join(append([]string{header}, rows...), "\n")
}

// logPaneHeader renders the pane's dim section label, marking a locked window so
// a stalled tail never reads as a stalled install.
func logPaneHeader(view logView, width int) string {
	label := "LOG"
	if view.locked {
		label = "LOG · LOCKED"
	}
	return lipgloss.NewStyle().Foreground(tui.ColorSlate500).MaxWidth(width).Render(label)
}

// renderLogTail renders the rows that ride under a step's own body when the
// frame is too narrow for a pane: the same stamped rows, led by a dim label.
func renderLogTail(src LogSource, view logView, width, budget int) []string {
	if src == nil || budget <= 0 {
		return nil
	}
	lines, first := src.Snapshot()
	rows := logRows(logWindow(lines, first, view, budget), width, budget, false)
	if len(rows) == 0 {
		return nil
	}
	return append([]string{logPaneHeader(view, width)}, rows...)
}

// renderLogFull renders the log across the whole body once `f` has swapped it
// full-screen: the same rows, sized to the body box instead of the pane.
func renderLogFull(src LogSource, view logView, width, height int) string {
	if src == nil {
		return ""
	}
	return renderLogPane(src, view, width, max(height, 2), true)
}

// lockedAt is the absolute stream index a fresh lock pins the window's end to:
// everything written so far.
func lockedAt(src LogSource) int64 {
	if src == nil {
		return 0
	}
	lines, first := src.Snapshot()
	return first + int64(len(lines))
}
