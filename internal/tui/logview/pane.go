package logview

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// stampFormat is the wall-clock stamp each log row leads with; seconds
// resolution is as fine as a step-by-step install needs.
const stampFormat = "15:04:05"

// levelError and levelWarn are the two severities the surface renders
// apart from the quiet info stream: a tag, a gutter mark, a tint, and a
// minimap cell each.
const (
	levelError = "ERROR"
	levelWarn  = "WARN"
)

// gutterWidth is the level gutter's one column plus the space separating it
// from the stamp; laneWidth is the minimap's single column at the window's
// right edge, and minimapMinWidth is the narrowest window that spends a
// column on it — below that the column is worth more as message text.
const (
	gutterWidth     = 2
	laneWidth       = 1
	minimapMinWidth = 24
)

// view is a log viewport's own state: where the window ends while follow is
// locked, and whether the log has taken the whole frame.
type view struct {
	locked bool
	// lockAt is the absolute stream index the locked window ends at (exclusive).
	lockAt int64
	full   bool
}

// window picks the rows a viewport shows from a ring snapshot — the tail
// while following, or the budget of lines ending at v.lockAt while locked,
// clamped to whatever the ring still holds — and reports the absolute stream
// index just past the window's last line.
func window(lines []Line, first int64, v view, budget int) (w []Line, end int64) {
	if budget <= 0 || len(lines) == 0 {
		return nil, first + int64(len(lines))
	}
	at := len(lines)
	if v.locked {
		at = min(max(int(v.lockAt-first), 0), len(lines))
	}
	// A lock older than everything the ring still holds shows the oldest rows it
	// has rather than nothing at all.
	if at == 0 {
		at = min(budget, len(lines))
	}
	return lines[max(at-budget, 0):at], first + int64(at)
}

// scroll moves v's window n lines through src's stream (negative is older),
// engaging the lock on the first move up and releasing it once the window's
// end returns to the tail — the standard pager contract. minLines floors the
// window's end so paging up stops with the stream's first line at the top of
// a full window, never past it.
func scroll(v *view, src Source, n, minLines int) {
	if src == nil || n == 0 {
		return
	}
	lines, first := src.Snapshot()
	total := first + int64(len(lines))
	at := total
	if v.locked {
		at = min(v.lockAt, total)
	}
	at += int64(n)
	at = max(at, min(first+int64(max(minLines, 1)), total))
	if at >= total {
		v.locked, v.lockAt = false, 0
		return
	}
	v.locked, v.lockAt = true, at
}

// visibleLines reports how many lines v's window is currently showing at
// width×height — the page one pgup/pgdn moves by.
func visibleLines(src Source, v view, width, height int, wrap bool) int {
	if src == nil {
		return 1
	}
	lines, first := src.Snapshot()
	budget := max(height-1, 1)
	w, _ := window(lines, first, v, budget)
	if wrap {
		w = fitWrapped(w, width, budget)
	}
	return max(len(w), 1)
}

// topLines reports how many of the stream's oldest lines fill one window at
// width×height — the floor scroll stops paging up at.
func topLines(src Source, width, height int, wrap bool) int {
	if src == nil {
		return 1
	}
	lines, _ := src.Snapshot()
	budget := max(height-1, 1)
	if !wrap {
		return max(min(budget, len(lines)), 1)
	}
	rows, n := 0, 0
	tw := textWidth(width)
	for i := range lines {
		rows += len(tui.WrapLines(lineText(&lines[i]), tw))
		if rows > budget {
			break
		}
		n++
	}
	return max(n, 1)
}

// fitWrapped trims w's oldest lines until the remainder wraps within budget
// rows at width, so the header's coordinates name only lines actually on
// screen; a single line taller than the whole budget stays, and renderRows
// clips its oldest rows as the backstop.
func fitWrapped(w []Line, width, budget int) []Line {
	tw := textWidth(width)
	rows := 0
	for i := len(w) - 1; i >= 0; i-- {
		rows += len(tui.WrapLines(lineText(&w[i]), tw))
		if rows > budget && i < len(w)-1 {
			return w[i+1:]
		}
	}
	return w
}

// renderRows renders lines as dim, stamped rows within width columns, never
// exceeding budget rows — the oldest are dropped first. A side pane and a narrow
// tail give one row per line and clip the overflow, since a wrapped line there
// costs a second row to show a few trailing fields; the full-screen log wraps
// instead, where there is room to read them.
func renderRows(lines []Line, width, budget int, wrap bool) []string {
	if width <= 0 || budget <= 0 {
		return nil
	}
	stampStyle := lipgloss.NewStyle().Foreground(tui.ColorSubtle())
	indent := gutterWidth + lipgloss.Width(stampFormat) + 1
	tw := textWidth(width)

	var rows []string
	for i := range lines {
		l := &lines[i]
		lead := levelStyle(l.Level).Render(levelGutter(l.Level)) + " " + stampStyle.Render(l.At.Format(stampFormat))
		textStyle := levelStyle(l.Level)
		text := lineText(l)
		if !wrap {
			rows = append(rows, lead+" "+textStyle.Render(tui.Truncate(text, tw)))
			continue
		}
		for j, part := range tui.WrapLines(text, tw) {
			if j == 0 {
				rows = append(rows, lead+" "+textStyle.Render(part))
				continue
			}
			rows = append(rows, strings.Repeat(" ", indent)+textStyle.Render(part))
		}
	}
	// A single line can still wrap taller than the whole budget, so the drop
	// happens after rendering: the newest rows are the ones worth keeping.
	return rows[max(len(rows)-budget, 0):]
}

// textWidth is the column budget a row's text gets beside its gutter and
// stamp, inside whatever the minimap lane left of the window.
func textWidth(width int) int {
	return max(rowWidth(width)-gutterWidth-lipgloss.Width(stampFormat)-1, 8)
}

// rowWidth is the columns a window's text rows render within: its own width
// less the minimap lane, on a window wide enough to carry one.
func rowWidth(width int) int {
	if !drawsMinimap(width) {
		return width
	}
	return width - laneWidth
}

// drawsMinimap reports whether a window of this width carries the lane.
func drawsMinimap(width int) bool {
	return width >= minimapMinWidth
}

// levelGutter returns the one-column severity mark a row leads with: the
// warning and error initials, a faint dot for the quiet info stream.
func levelGutter(level string) string {
	switch strings.ToUpper(level) {
	case levelError:
		return "E"
	case levelWarn:
		return "W"
	default:
		return tui.IconLevelInfo
	}
}

// lineText returns the one line a Line renders as: the WARN/ERROR tag, then
// the message with its fields.
func lineText(l *Line) string {
	if tag := levelTag(l.Level); tag != "" {
		return tag + " " + l.Text
	}
	return l.Text
}

// levelTag returns the textual severity a rendered row leads with — WARN
// and ERROR only, so severity survives NO_COLOR without tagging the whole
// dim info stream.
func levelTag(level string) string {
	switch upper := strings.ToUpper(level); upper {
	case levelError, levelWarn:
		return upper
	default:
		return ""
	}
}

// levelStyle returns the colour a captured line renders in: a warning and a
// failure must stand out of the dim stream, since the failure screen's evidence
// is whatever the tail was carrying when the run stopped.
func levelStyle(level string) lipgloss.Style {
	switch strings.ToUpper(level) {
	case levelError:
		return lipgloss.NewStyle().Foreground(tui.ColorError())
	case levelWarn:
		return lipgloss.NewStyle().Foreground(tui.ColorWarning())
	default:
		return lipgloss.NewStyle().Foreground(tui.ColorTextDim())
	}
}

// renderPane renders a log viewport into at most height rows of width
// columns: a dim header naming the follow state, then the window's stamped rows,
// tail last. wrap is the full-screen view's line handling; see renderRows.
func renderPane(src Source, v view, width, height int, wrap bool) string {
	if src == nil || height <= 0 {
		return ""
	}
	budget := max(height-1, 1)
	lines, first := src.Snapshot()
	w, end := window(lines, first, v, budget)
	if wrap {
		w = fitWrapped(w, width, budget)
	}
	header := paneHeader(v, len(w), end, first+int64(len(lines)), width)
	rows := renderRows(w, width, budget, wrap)
	if len(rows) == 0 {
		rows = []string{lipgloss.NewStyle().Foreground(tui.ColorSubtle()).Render("waiting for the first log line…")}
	}
	rows = withMinimap(rows, lines, width, int(end-first)-len(w), int(end-first))
	return strings.Join(append([]string{header}, rows...), "\n")
}

// withMinimap pads each of rows out to the lane column and appends its own
// cell. rows come back untouched on a window too narrow to spend a column
// on, and on one already showing the whole stream — a lane over a stream
// with nothing off screen would mark scroll targets that are already read.
func withMinimap(rows []string, lines []Line, width, winStart, winEnd int) []string {
	if !drawsMinimap(width) || len(rows) == 0 || len(lines) == 0 {
		return rows
	}
	if winStart <= 0 && winEnd >= len(lines) {
		return rows
	}
	lane := minimapLane(lines, len(rows), winStart, winEnd)
	pad := rowWidth(width)
	out := make([]string, len(rows))
	for i, row := range rows {
		gap := max(pad-lipgloss.Width(row), 0)
		out[i] = row + strings.Repeat(" ", gap) + lane[i]
	}
	return out
}

// minimapLane maps the stream's whole index range onto rows lane cells, so a
// scroll target is visible before the operator scrolls: a cell marks the
// worst level any line in its bucket carried, and the cells the visible
// window covers read as a thumb against the track. winStart and winEnd bound
// the visible window in lines' own index space, end exclusive.
func minimapLane(lines []Line, rows, winStart, winEnd int) []string {
	track := lipgloss.NewStyle().Foreground(tui.ColorRule()).Render(tui.IconBarSegment)
	thumb := lipgloss.NewStyle().Foreground(tui.ColorSubtle()).Render(tui.IconBarTick)
	warn := lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render(tui.IconBar)
	fail := lipgloss.NewStyle().Foreground(tui.ColorError()).Render(tui.IconBar)

	lane := make([]string, rows)
	for i := range rows {
		lo, hi := i*len(lines)/rows, (i+1)*len(lines)/rows
		switch level := worstLevel(lines[lo:hi]); {
		case level == levelError:
			lane[i] = fail
		case level == levelWarn:
			lane[i] = warn
		case lo < winEnd && max(hi, lo+1) > winStart:
			lane[i] = thumb
		default:
			lane[i] = track
		}
	}
	return lane
}

// worstLevel names the loudest severity in one minimap bucket, empty when
// the bucket holds nothing louder than info.
func worstLevel(bucket []Line) string {
	worst := ""
	for i := range bucket {
		switch strings.ToUpper(bucket[i].Level) {
		case levelError:
			return levelError
		case levelWarn:
			worst = levelWarn
		}
	}
	return worst
}

// paneHeader renders the pane's dim section label; a locked window names
// the shown lines' span out of the stream's total — "LOG · 212–260 of 412" —
// so a stalled tail reads as the paused pager it is, and paging always says
// where it stands.
func paneHeader(v view, shown int, end, total int64, width int) string {
	label := "LOG"
	if v.locked && shown > 0 {
		label = fmt.Sprintf("LOG · %d–%d of %d", end-int64(shown)+1, end, total)
	}
	return lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).MaxWidth(width).Render(label)
}

// renderTail renders the rows that ride under a step's own body when the
// frame is too narrow for a pane: the same stamped rows, led by a dim label.
func renderTail(src Source, v view, width, budget int) []string {
	if src == nil || budget <= 0 {
		return nil
	}
	lines, first := src.Snapshot()
	w, end := window(lines, first, v, budget)
	rows := renderRows(w, width, budget, false)
	if len(rows) == 0 {
		return nil
	}
	rows = withMinimap(rows, lines, width, int(end-first)-len(w), int(end-first))
	return append([]string{paneHeader(v, len(w), end, first+int64(len(lines)), width)}, rows...)
}

// renderFull renders the log across the whole body once `f` has swapped it
// full-screen: the same rows, sized to the body box instead of the pane.
func renderFull(src Source, v view, width, height int) string {
	if src == nil {
		return ""
	}
	return renderPane(src, v, width, max(height, 2), true)
}

// lockedAt is the absolute stream index a fresh lock pins the window's end to:
// everything written so far.
func lockedAt(src Source) int64 {
	if src == nil {
		return 0
	}
	lines, first := src.Snapshot()
	return first + int64(len(lines))
}
