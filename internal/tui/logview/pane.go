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
// locked, whether the log has taken the whole frame, and the live filter.
type view struct {
	locked bool
	// lockAt is the absolute stream index the locked window ends at (exclusive).
	lockAt int64
	full   bool
	filter filter
}

// windowIn picks the rows a viewport shows from one already-selected
// sequence, reporting the window's end position in the sequence's own
// coordinates alongside the absolute stream index just past its last line.
func windowIn(st *stream, v view, budget int) (w []Line, pos int, end int64) {
	if budget <= 0 || st.len() == 0 {
		return nil, 0, 0
	}
	at := st.len()
	if v.locked {
		at = min(max(st.below(v.lockAt), 0), st.len())
	}
	// A lock older than everything the sequence still holds shows the oldest
	// rows it has rather than nothing at all.
	if at == 0 {
		at = min(budget, st.len())
	}
	return st.lines[max(at-budget, 0):at], at, st.at(at-1) + 1
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
	st := v.filter.selectFrom(lines, first)
	if st.len() == 0 {
		return
	}
	at := st.len()
	if v.locked {
		at = min(max(st.below(v.lockAt), 0), st.len())
	}
	at += n
	at = max(at, min(max(minLines, 1), st.len()))
	if at >= st.len() {
		v.locked, v.lockAt = false, 0
		return
	}
	v.locked, v.lockAt = true, st.at(at-1)+1
}

// jump moves the window's end to just past the next line of interest after
// it (dir positive) or the last one before its final row (dir negative),
// releasing the lock when the target is the sequence's own tail. Nothing
// moves when there is no such line.
func jump(v *view, src Source, dir int) {
	if src == nil || dir == 0 {
		return
	}
	lines, first := src.Snapshot()
	if len(lines) == 0 {
		return
	}
	end := first + int64(len(lines))
	if v.locked {
		end = min(max(v.lockAt, first), end)
	}
	from := int(end - first)
	if dir < 0 {
		from -= 2
	}
	for i := from; i >= 0 && i < len(lines); i += dir {
		if !v.filter.interesting(&lines[i]) {
			continue
		}
		if target := first + int64(i) + 1; target >= first+int64(len(lines)) {
			v.locked, v.lockAt = false, 0
		} else {
			v.locked, v.lockAt = true, target
		}
		return
	}
}

// visibleLines reports how many lines v's window is currently showing at
// width×height — the page one pgup/pgdn moves by.
func visibleLines(src Source, v view, width, height int, wrap bool) int {
	if src == nil {
		return 1
	}
	lines, first := src.Snapshot()
	budget := max(height-1, 1)
	st := v.filter.selectFrom(lines, first)
	w, _, _ := windowIn(&st, v, budget)
	if wrap {
		w = fitWrapped(w, width, budget)
	}
	return max(len(w), 1)
}

// topLines reports how many of the stream's oldest lines fill one window at
// width×height — the floor scroll stops paging up at.
func topLines(src Source, v view, width, height int, wrap bool) int {
	if src == nil {
		return 1
	}
	lines, first := src.Snapshot()
	st := v.filter.selectFrom(lines, first)
	budget := max(height-1, 1)
	if !wrap {
		return max(min(budget, st.len()), 1)
	}
	rows, n := 0, 0
	tw := textWidth(width)
	for i := range st.lines {
		rows += len(tui.WrapLines(lineText(&st.lines[i]), tw))
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
// exceeding budget rows — the oldest are dropped first. The tail gives one
// row per line and clips the overflow, since a wrapped line there costs a
// second row to show a few trailing fields; the full-screen log wraps
// instead, where there is room to read them.
func renderRows(lines []Line, width, budget int, wrap bool) []string {
	if width <= 0 || budget <= 0 {
		return nil
	}
	stampStyle := lipgloss.NewStyle().Foreground(tui.ColorTextFaint())
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

// renderFull renders the log across the whole body once `f` has swapped it
// full-screen, into at most height rows of width columns: a dim header naming
// the follow state, then the window's stamped, wrapped rows, tail last.
func renderFull(src Source, v view, width, height int) string {
	if src == nil {
		return ""
	}
	height = max(height, 2)
	budget := max(height-1, 1)
	lines, first := src.Snapshot()
	st := v.filter.selectFrom(lines, first)
	w, pos, end := windowIn(&st, v, budget)
	w = fitWrapped(w, width, budget)
	header := paneHeader(v, coords{
		shown: len(w), end: end, total: first + int64(len(lines)),
		pos: pos, matches: st.len(),
	}, width)
	rows := renderRows(w, width, budget, true)
	if len(rows) == 0 {
		rows = []string{lipgloss.NewStyle().Foreground(tui.ColorSubtle()).Render(emptyNote(v.filter))}
	}
	rows = withMinimap(rows, lines, width, windowStart(&st, w, pos, first), int(end-first))
	return strings.Join(append([]string{header}, rows...), "\n")
}

// windowStart is the raw stream index — relative to first, as withMinimap
// wants it — of the first line w shows. w's raw-index span equals its own
// length only when every line between pos-len(w) and pos survived the
// filter; a sparse filter's matches are not contiguous in raw index space,
// so the true start is read off st (which remembers each surviving member's
// own raw index) rather than assumed from len(w).
func windowStart(st *stream, w []Line, pos int, first int64) int {
	if len(w) == 0 {
		return 0
	}
	return int(st.at(pos-len(w)) - first)
}

// withMinimap pads each of rows out to the lane column and appends its own
// cell. rows come back untouched on a window too narrow to spend a column
// on, on one already showing the whole stream — a lane over a stream with
// nothing off screen would mark scroll targets that are already read — and on
// one whose only row is a placeholder note, which sits in no position at all.
func withMinimap(rows []string, lines []Line, width, winStart, winEnd int) []string {
	if !drawsMinimap(width) || len(rows) == 0 || len(lines) == 0 || winEnd <= winStart {
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

// coords is what the pane header counts with: the absolute stream span the
// window ends at, and the window's own position among a filter's matches.
type coords struct {
	shown        int
	end, total   int64
	pos, matches int
}

// paneHeader renders the pane's dim section label, naming the space it counts
// in. With no filter a locked window names the shown lines' absolute span out
// of the stream's total — "LOG · 212–260 of 412" — so a stalled tail reads as
// the paused pager it is. With a filter live the chip carries the pattern and
// its match count, and a locked window's span is stated in matches instead,
// since the absolute line numbers of a filtered window are not contiguous.
func paneHeader(v view, c coords, width int) string {
	parts := []string{"LOG"}
	if chip := filterChip(v.filter, c.matches, int(c.total)); chip != "" {
		parts = append(parts, chip)
	}
	switch {
	case c.shown == 0 || !v.locked:
	case v.filter.active():
		parts = append(parts, fmt.Sprintf("match %d–%d of %d", c.pos-c.shown+1, c.pos, c.matches))
	default:
		parts = append(parts, fmt.Sprintf("%d–%d of %d", c.end-int64(c.shown)+1, c.end, c.total))
	}
	return lipgloss.NewStyle().Foreground(tui.ColorAccent()).Bold(true).MaxWidth(width).Render(strings.Join(parts, " · "))
}

// emptyNote is what a window with no rows says: an unfiltered one is still
// waiting for output, a filtered one has excluded everything there is.
func emptyNote(f filter) string {
	if f.engaged() {
		return "no log line matches this filter"
	}
	return "waiting for the first log line…"
}

// filterChip renders the persistent filter chip — the pattern as typed with
// its match count out of the stream's total. While the pattern is still being
// typed the chip leads with the "/" that opened it, so the input mode reads
// without relying on colour.
func filterChip(f filter, matches, total int) string {
	if !f.active() && !f.typing {
		return ""
	}
	label := "filter: " + f.text
	if f.typing {
		label = "/" + f.text
	}
	return fmt.Sprintf("%s %s (%d/%d)", tui.IconCaretRight, label, matches, total)
}

// renderTail renders the rows that ride under a step's own body: the same
// stamped rows, led by a dim label.
func renderTail(src Source, v view, width, budget int) []string {
	if src == nil || budget <= 0 {
		return nil
	}
	lines, first := src.Snapshot()
	st := v.filter.selectFrom(lines, first)
	w, pos, end := windowIn(&st, v, budget)
	rows := renderRows(w, width, budget, false)
	if len(rows) == 0 {
		if !v.filter.engaged() {
			return nil
		}
		// A filter matching nothing must not take the chip down with the rows
		// — it is the only thing on screen explaining why they are gone.
		rows = []string{lipgloss.NewStyle().Foreground(tui.ColorSubtle()).Render(emptyNote(v.filter))}
	}
	rows = withMinimap(rows, lines, width, windowStart(&st, w, pos, first), int(end-first))
	header := paneHeader(v, coords{
		shown: len(w), end: end, total: first + int64(len(lines)),
		pos: pos, matches: st.len(),
	}, width)
	return append([]string{header}, rows...)
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
