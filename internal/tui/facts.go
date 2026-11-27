package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// FactRow is one key/value fact for RenderFacts; Link wraps the value in an
// OSC 8 hyperlink that degrades to plain text off-TTY.
type FactRow struct {
	Key, Value string
	Link       string
	Highlight  bool
	Sub        bool
}

// FactLeader selects what joins a fact's key to its value.
type FactLeader int

// Fact leader dialects: dotted leaders, a padded key column, and flowing
// "key: value".
const (
	FactLeaderDots FactLeader = iota
	FactLeaderPad
	FactLeaderColon
)

// FactStyles is the style set RenderFacts paints one dialect with.
type FactStyles struct {
	Key, SubKey, Value, Highlight, Leader lipgloss.Style
}

// DefaultFactStyles returns the dotted dialect's styles from the active theme.
func DefaultFactStyles() FactStyles {
	return FactStyles{
		Key:       lipgloss.NewStyle().Foreground(ColorTextDim()),
		SubKey:    lipgloss.NewStyle().Foreground(ColorTextFaint()),
		Value:     lipgloss.NewStyle(),
		Highlight: lipgloss.NewStyle().Foreground(ColorWarning()).Bold(true),
		Leader:    lipgloss.NewStyle().Foreground(ColorRule()),
	}
}

// FactLayout shapes one RenderFacts call; a zero KeyWidth means
// DefaultKeyColWidth and a zero TotalWidth disables wrapping.
type FactLayout struct {
	Leader     FactLeader
	KeyWidth   int
	TotalWidth int
	Styles     FactStyles
}

// RenderFacts renders rows as styled fact lines in one dialect. Lines come
// back un-downsampled.
func RenderFacts(rows []FactRow, l *FactLayout) []string {
	var lines []string
	for i := range rows {
		lines = append(lines, renderFactRow(&rows[i], l)...)
	}
	return lines
}

func renderFactRow(row *FactRow, l *FactLayout) []string {
	keyStyle := l.Styles.Key
	if row.Sub {
		keyStyle = l.Styles.SubKey
	}
	valueStyle := l.Styles.Value
	if row.Highlight {
		valueStyle = l.Styles.Highlight
	}
	if l.Leader == FactLeaderColon {
		return renderColonFact(row, l, &keyStyle, &valueStyle)
	}

	keyColWidth := l.KeyWidth
	if keyColWidth <= 0 {
		keyColWidth = DefaultKeyColWidth
	}

	var prefix string
	var valueStart int
	switch l.Leader {
	case FactLeaderPad:
		// A key style carrying its own Width has already padded to the column.
		styled := keyStyle.Render(row.Key)
		pad := keyColWidth - lipgloss.Width(styled)
		if pad < 1 && lipgloss.Width(styled) >= keyColWidth && keyStyle.GetWidth() == 0 {
			pad = 1
		}
		pad = max(pad, 0)
		valueStart = lipgloss.Width(styled) + pad
		prefix = styled + strings.Repeat(" ", pad)
	default: // FactLeaderDots
		keyLen := lipgloss.Width(row.Key)
		// A key overrunning the column shifts only its own row.
		dotsNeeded := keyColWidth - keyLen - 2
		if dotsNeeded < 1 {
			dotsNeeded = 3
		}
		valueStart = keyLen + 1 + dotsNeeded + 1
		prefix = keyStyle.Render(row.Key) + " " + l.Styles.Leader.Render(strings.Repeat(".", dotsNeeded)) + " "
	}

	return strings.Split(prefix+wrapValueColumn(row.Value, valueStart, &valueStyle, l.TotalWidth, row.Link), "\n")
}

// renderColonFact wraps the raw line first and styles after, since styling
// before wrapping would split ANSI sequences.
func renderColonFact(row *FactRow, l *FactLayout, keyStyle, valueStyle *lipgloss.Style) []string {
	raw := row.Key + ": " + row.Value
	wrapped := raw
	if l.TotalWidth > 0 {
		wrapped = lipgloss.Wrap(raw, l.TotalWidth, "")
	}

	keyLen := len([]rune(row.Key + ":"))
	consumed := 0
	var out []string
	for line := range strings.SplitSeq(wrapped, "\n") {
		runes := []rune(line)
		switch {
		case consumed >= keyLen:
			out = append(out, valueStyle.Render(line))
		case len(runes) <= keyLen-consumed:
			out = append(out, keyStyle.Render(line))
		default:
			seam := keyLen - consumed
			out = append(out, keyStyle.Render(string(runes[:seam]))+valueStyle.Render(string(runes[seam:])))
		}
		consumed += len(runes)
	}
	return out
}
