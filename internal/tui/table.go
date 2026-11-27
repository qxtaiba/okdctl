package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Align selects a column's horizontal cell alignment.
type Align int

// Column alignments.
const (
	AlignLeft Align = iota
	AlignRight
)

// Column describes one table column.
type Column struct {
	Header string
	Align  Align
	// MinWidth floors the column; MaxWidth caps it (0 = uncapped)
	MinWidth int
	MaxWidth int
	// Weight is the column's share of TableOptions.Width surplus; 0 keeps the
	// natural width
	Weight int
}

// RowGroup is a run of rows under a dim title line; an empty Title renders
// the rows alone.
type RowGroup struct {
	Title string
	Rows  [][]string
}

// TableOptions configures Table and ColumnTable; the zero value renders a
// plain table with a two-space gap.
type TableOptions struct {
	// RowStyle styles a whole data row (0-indexed, continuous across groups)
	// when it returns true
	RowStyle func(row int) (lipgloss.Style, bool)
	// MaxColWidth middle-truncates wider cells; zero disables
	MaxColWidth int
	Gap         int
	PlainHeader bool
	// Width is the budget Column.Weight distributes surplus against; zero
	// ignores weights
	Width int
}

// Table renders rows as aligned lines, header first. Lines come back
// un-downsampled; callers outside a Boxed* helper Downsample them.
func Table(headers []string, rows [][]string, opts TableOptions) []string {
	cols := make([]Column, len(headers))
	for i, h := range headers {
		cols[i] = Column{Header: h}
	}
	return ColumnTable(cols, []RowGroup{{Rows: rows}}, opts)
}

// ColumnTable renders a column-spec table as lines, each group's dim title
// before its rows. Lines come back un-downsampled, like Table's.
func ColumnTable(cols []Column, groups []RowGroup, opts TableOptions) []string {
	gap := opts.Gap
	if gap == 0 {
		gap = 2
	}

	widths := columnWidths(cols, groups, opts)

	lineCount := 1
	for _, g := range groups {
		lineCount += len(g.Rows)
		if g.Title != "" {
			lineCount++
		}
	}
	lines := make([]string, 0, lineCount)

	header := renderTableRow(headerCells(cols), cols, widths, opts, gap)
	if !opts.PlainHeader {
		header = DimStyle.Render(header)
	}
	lines = append(lines, header)

	dataRow := 0
	for _, g := range groups {
		if g.Title != "" {
			lines = append(lines, DimStyle.Bold(true).Render(g.Title))
		}
		for _, row := range g.Rows {
			line := renderTableRow(row, cols, widths, opts, gap)
			if opts.RowStyle != nil {
				if st, ok := opts.RowStyle(dataRow); ok {
					line = st.Render(line)
				}
			}
			lines = append(lines, line)
			dataRow++
		}
	}
	return lines
}

// columnWidths sizes each column to its widest capped cell within its
// bounds, then shares any Width surplus by weight.
func columnWidths(cols []Column, groups []RowGroup, opts TableOptions) []int {
	widths := make([]int, len(cols))
	measure := func(row []string) {
		for c := range min(len(cols), len(row)) {
			cell := truncateMiddle(row[c], cellCap(&cols[c], opts))
			widths[c] = max(widths[c], lipgloss.Width(cell))
		}
	}
	measure(headerCells(cols))
	for _, g := range groups {
		for _, row := range g.Rows {
			measure(row)
		}
	}
	for c := range cols {
		widths[c] = max(widths[c], cols[c].MinWidth)
	}

	if opts.Width <= 0 {
		return widths
	}
	gap := opts.Gap
	if gap == 0 {
		gap = 2
	}
	total := gap * (len(cols) - 1)
	weightSum := 0
	for c := range cols {
		total += widths[c]
		weightSum += cols[c].Weight
	}
	surplus := opts.Width - total
	if surplus <= 0 || weightSum == 0 {
		return widths
	}
	given := 0
	for c := range cols {
		widths[c] += surplus * cols[c].Weight / weightSum
		given += surplus * cols[c].Weight / weightSum
	}
	for c := range cols {
		if given == surplus {
			break
		}
		if cols[c].Weight > 0 {
			widths[c]++
			given++
		}
	}
	return widths
}

func cellCap(col *Column, opts TableOptions) int {
	switch {
	case col.MaxWidth > 0 && opts.MaxColWidth > 0:
		return min(col.MaxWidth, opts.MaxColWidth)
	case col.MaxWidth > 0:
		return col.MaxWidth
	default:
		return opts.MaxColWidth
	}
}

func headerCells(cols []Column) []string {
	cells := make([]string, len(cols))
	for i := range cols {
		cells[i] = cols[i].Header
	}
	return cells
}

func renderTableRow(row []string, cols []Column, widths []int, opts TableOptions, gap int) string {
	parts := make([]string, len(cols))
	for c := range cols {
		var cell string
		if c < len(row) {
			cell = truncateMiddle(row[c], cellCap(&cols[c], opts))
		}
		if cols[c].Align == AlignRight {
			parts[c] = padLeftCells(cell, widths[c])
		} else {
			parts[c] = padRightCells(cell, widths[c])
		}
	}
	return strings.Join(parts, strings.Repeat(" ", gap))
}

func padRightCells(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func padLeftCells(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// Truncate clips s grapheme-safely to maxW columns, appending "…" when it
// clips.
func Truncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxW {
		return s
	}
	return ansi.Truncate(foldInvalidUTF8(s), maxW, "…")
}
