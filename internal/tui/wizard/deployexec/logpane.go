package deployexec

import (
	"fmt"
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

// logWindow picks the rows a viewport shows from a ring snapshot — the tail
// while following, or the budget of lines ending at view.lockAt while locked,
// clamped to whatever the ring still holds — and reports the absolute stream
// index just past the window's last line.
func logWindow(lines []LogLine, first int64, view logView, budget int) (window []LogLine, end int64) {
	if budget <= 0 || len(lines) == 0 {
		return nil, first + int64(len(lines))
	}
	at := len(lines)
	if view.locked {
		at = min(max(int(view.lockAt-first), 0), len(lines))
	}
	// A lock older than everything the ring still holds shows the oldest rows it
	// has rather than nothing at all.
	if at == 0 {
		at = min(budget, len(lines))
	}
	return lines[max(at-budget, 0):at], first + int64(at)
}

// scrollLog moves view's window n lines through src's stream (negative is
// older), engaging the lock on the first move up and releasing it once the
// window's end returns to the tail — the standard pager contract. minLines
// floors the window's end so paging up stops with the stream's first line at
// the top of a full window, never past it.
func scrollLog(view *logView, src LogSource, n, minLines int) {
	if src == nil || n == 0 {
		return
	}
	lines, first := src.Snapshot()
	total := first + int64(len(lines))
	at := total
	if view.locked {
		at = min(view.lockAt, total)
	}
	at += int64(n)
	at = max(at, min(first+int64(max(minLines, 1)), total))
	if at >= total {
		view.locked, view.lockAt = false, 0
		return
	}
	view.locked, view.lockAt = true, at
}

// visibleLogLines reports how many lines view's window is currently showing
// at width×height — the page one pgup/pgdn moves by.
func visibleLogLines(src LogSource, view logView, width, height int, wrap bool) int {
	if src == nil {
		return 1
	}
	lines, first := src.Snapshot()
	budget := max(height-1, 1)
	window, _ := logWindow(lines, first, view, budget)
	if wrap {
		window = fitWrapped(window, width, budget)
	}
	return max(len(window), 1)
}

// topLogLines reports how many of the stream's oldest lines fill one window
// at width×height — the floor scrollLog stops paging up at.
func topLogLines(src LogSource, width, height int, wrap bool) int {
	if src == nil {
		return 1
	}
	lines, _ := src.Snapshot()
	budget := max(height-1, 1)
	if !wrap {
		return max(min(budget, len(lines)), 1)
	}
	rows, n := 0, 0
	textWidth := logTextWidth(width)
	for i := range lines {
		rows += len(tui.WrapLines(logLineText(&lines[i]), textWidth))
		if rows > budget {
			break
		}
		n++
	}
	return max(n, 1)
}

// fitWrapped trims window's oldest lines until the remainder wraps within
// budget rows at width, so the header's coordinates name only lines actually
// on screen; a single line taller than the whole budget stays, and logRows
// clips its oldest rows as the backstop.
func fitWrapped(window []LogLine, width, budget int) []LogLine {
	textWidth := logTextWidth(width)
	rows := 0
	for i := len(window) - 1; i >= 0; i-- {
		rows += len(tui.WrapLines(logLineText(&window[i]), textWidth))
		if rows > budget && i < len(window)-1 {
			return window[i+1:]
		}
	}
	return window
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
	textWidth := logTextWidth(width)

	var rows []string
	for i := range lines {
		l := &lines[i]
		stamp := stampStyle.Render(l.At.Format(logStampFormat))
		textStyle := logLevelStyle(l.Level)
		text := logLineText(l)
		if !wrap {
			rows = append(rows, stamp+" "+textStyle.Render(tui.Truncate(text, textWidth)))
			continue
		}
		for j, part := range tui.WrapLines(text, textWidth) {
			if j == 0 {
				rows = append(rows, stamp+" "+textStyle.Render(part))
				continue
			}
			rows = append(rows, strings.Repeat(" ", stampWidth+1)+textStyle.Render(part))
		}
	}
	// A single line can still wrap taller than the whole budget, so the drop
	// happens after rendering: the newest rows are the ones worth keeping.
	return rows[max(len(rows)-budget, 0):]
}

// logTextWidth is the column budget a row's text gets beside its stamp.
func logTextWidth(width int) int {
	return max(width-lipgloss.Width(logStampFormat)-1, 8)
}

// logLineText returns the one line a LogLine renders as: the WARN/ERROR tag,
// then the message with its fields.
func logLineText(l *LogLine) string {
	if tag := logLevelTag(l.Level); tag != "" {
		return tag + " " + l.Text
	}
	return l.Text
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
	budget := max(height-1, 1)
	lines, first := src.Snapshot()
	window, end := logWindow(lines, first, view, budget)
	if wrap {
		window = fitWrapped(window, width, budget)
	}
	header := logPaneHeader(view, len(window), end, first+int64(len(lines)), width)
	rows := logRows(window, width, budget, wrap)
	if len(rows) == 0 {
		rows = []string{lipgloss.NewStyle().Foreground(tui.ColorSlate600).Render("waiting for the first log line…")}
	}
	return strings.Join(append([]string{header}, rows...), "\n")
}

// logPaneHeader renders the pane's dim section label; a locked window names
// the shown lines' span out of the stream's total — "LOG · 212–260 of 412" —
// so a stalled tail reads as the paused pager it is, and paging always says
// where it stands.
func logPaneHeader(view logView, shown int, end, total int64, width int) string {
	label := "LOG"
	if view.locked && shown > 0 {
		label = fmt.Sprintf("LOG · %d–%d of %d", end-int64(shown)+1, end, total)
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
	window, end := logWindow(lines, first, view, budget)
	rows := logRows(window, width, budget, false)
	if len(rows) == 0 {
		return nil
	}
	return append([]string{logPaneHeader(view, len(window), end, first+int64(len(lines)), width)}, rows...)
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
