package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// TableOptions configures Table; the zero value renders a plain table with
// a two-space gap.
type TableOptions struct {
	// RowStyle is consulted per data row (0-indexed, excluding header); a
	// true return styles the whole row. Padding is computed on plain text
	// so escapes never shift columns.
	RowStyle func(row int) (lipgloss.Style, bool)
	// MaxColWidth middle-truncates any cell wider than the cap with an
	// ellipsis; zero disables truncation.
	MaxColWidth int
	// Gap is the number of spaces between columns; zero defaults to 2.
	Gap int
	// PlainHeader leaves the header unstyled; by default it renders dim.
	PlainHeader bool
}

// Table renders an aligned column table as lines (header then one per row),
// widths sized to the widest plain cell. Lines come back un-downsampled —
// callers outside a Boxed* helper must Downsample each line themselves.
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
		for c := range min(cols, len(row)) {
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
// ellipsis, preserving the distinguishing head and tail; maxW <= 0 returns s
// unchanged.
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
