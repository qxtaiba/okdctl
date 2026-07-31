package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// TableOptions configures Table. The zero value renders a plain, uncolored
// table with a two-space column gap.
type TableOptions struct {
	// RowStyle, when set, is consulted per data row (0-indexed, excluding the
	// header). A true second return applies the style to the whole row —
	// used to paint a not-ready node red. Padding is always computed on the
	// plain cell text, so the style's zero-width escapes never shift columns.
	RowStyle func(row int) (lipgloss.Style, bool)
	// MaxColWidth middle-truncates any cell wider than the cap with an
	// ellipsis so one long value cannot blow the width budget. Zero disables
	// truncation.
	MaxColWidth int
	// Gap is the number of spaces between columns; zero defaults to 2.
	Gap int
	// PlainHeader leaves the header unstyled; by default it renders in the dim
	// text color.
	PlainHeader bool
}

// Table renders an aligned column table as a slice of lines: a header row
// followed by one line per data row. Column widths come from the widest plain
// cell (header included). It is the single table look shared by status and
// node list; the node-op boxes keep their dotted key/value node lines, whose
// nested annotations and full terraform addresses don't fit flat columns.
// Lines come back un-downsampled — the Boxed* helpers gate embedded tables,
// callers printing outside a box must Downsample each line.
func Table(headers []string, rows [][]string, opts TableOptions) []string {
	gap := opts.Gap
	if gap == 0 {
		gap = 2
	}
	cols := len(headers)

	cells := make([][]string, 0, len(rows)+1)
	cells = append(cells, headers)
	cells = append(cells, rows...)

	widths := make([]int, cols)
	for _, row := range cells {
		for c := 0; c < cols && c < len(row); c++ {
			cell := truncateMiddle(row[c], opts.MaxColWidth)
			widths[c] = max(widths[c], lipgloss.Width(cell))
		}
	}

	sep := strings.Repeat(" ", gap)
	lines := make([]string, 0, len(cells))
	for ri, row := range cells {
		parts := make([]string, cols)
		for c := range cols {
			var cell string
			if c < len(row) {
				cell = truncateMiddle(row[c], opts.MaxColWidth)
			}
			parts[c] = padRightCells(cell, widths[c])
		}
		line := strings.Join(parts, sep)

		switch {
		case ri == 0 && !opts.PlainHeader:
			line = DimStyle.Render(line)
		case ri > 0 && opts.RowStyle != nil:
			if st, ok := opts.RowStyle(ri - 1); ok {
				line = st.Render(line)
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func padRightCells(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// truncateMiddle shortens s to maxW columns by replacing the middle with an
// ellipsis, preserving the distinguishing head and tail (a terraform address
// tail, a node-name suffix). maxW <= 0 returns s unchanged.
func truncateMiddle(s string, maxW int) string {
	if maxW <= 0 || lipgloss.Width(s) <= maxW {
		return s
	}
	if maxW <= 1 {
		return "…"
	}
	keep := maxW - 1
	head := keep / 2
	tail := keep - head
	return s[:head] + "…" + s[len(s)-tail:]
}
