package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Align selects a column's horizontal cell alignment.
type Align int

// Column alignments; numeric columns read best right-aligned.
const (
	AlignLeft Align = iota
	AlignRight
)

// Column describes one table column: its header, alignment, width bounds,
// and its share of surplus width under TableOptions.Width.
type Column struct {
	Header string
	Align  Align
	// MinWidth floors the column; MaxWidth caps it, middle-truncating wider
	// cells (0 = natural/uncapped).
	MinWidth int
	MaxWidth int
	// Weight is the column's proportional share of the surplus once
	// TableOptions.Width exceeds the table's natural width; 0 keeps the
	// column at its natural width.
	Weight int
}

// RowGroup is a run of rows rendered under a dim group-title line; an empty
// Title renders the rows with no title line.
type RowGroup struct {
	Title string
	Rows  [][]string
}

// TableOptions configures Table and ColumnTable; the zero value renders a
// plain table with a two-space gap.
type TableOptions struct {
	// RowStyle is consulted per data row (0-indexed, excluding header and
	// group-title lines, continuous across groups); a true return styles the
	// whole row. Padding is computed on plain text so escapes never shift
	// columns.
	RowStyle func(row int) (lipgloss.Style, bool)
	// MaxColWidth middle-truncates any cell wider than the cap with an
	// ellipsis; zero disables truncation.
	MaxColWidth int
	// Gap is the number of spaces between columns; zero defaults to 2.
	Gap int
	// PlainHeader leaves the header unstyled; by default it renders dim.
	PlainHeader bool
	// Width is the total budget Column.Weight distributes surplus against;
	// zero renders at natural width and ignores weights.
	Width int
}

// Table renders an aligned column table as lines (header then one per row),
// widths sized to the widest plain cell. Lines come back un-downsampled —
// callers outside a Boxed* helper must Downsample each line themselves.
func Table(headers []string, rows [][]string, opts TableOptions) []string {
	cols := make([]Column, len(headers))
	for i, h := range headers {
		cols[i] = Column{Header: h}
	}
	return ColumnTable(cols, []RowGroup{{Rows: rows}}, opts)
}

// ColumnTable renders a column-spec table as lines: the header, then each
// group's dim title line followed by its rows. Lines come back
// un-downsampled, like Table's.
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

// columnWidths sizes each column to its widest capped cell, clamps to the
// column's own Min/MaxWidth, then hands any Width surplus to the weighted
// columns proportionally (remainder left to right).
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

// cellCap is the effective per-cell truncation cap for a column: the tighter
// of the table-wide MaxColWidth and the column's own MaxWidth.
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

// Truncate rune-safely clips s to fit within maxW visible columns, appending
// "…" when it clips.
func Truncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxW {
		return s
	}
	runes := []rune(s)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := string(runes[:i]) + "…"
		if lipgloss.Width(candidate) <= maxW {
			return candidate
		}
	}
	return "…"
}
