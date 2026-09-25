package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// FactRow is one key/value fact for RenderFacts; Sub marks a nested key
// (one tier fainter) and Highlight an amber-emphasized value.
type FactRow struct {
	Key, Value string
	Highlight  bool
	Sub        bool
}

// FactLeader selects what joins a fact's key to its value.
type FactLeader int

// Fact leader dialects: dotted leaders (done cards, CLI summaries), a padded
// key column (review sections), and flowing "key: value" (the context pane).
const (
	FactLeaderDots FactLeader = iota
	FactLeaderPad
	FactLeaderColon
)

// FactStyles is the style set RenderFacts paints one dialect with.
type FactStyles struct {
	Key, SubKey, Value, Highlight, Leader lipgloss.Style
}

// DefaultFactStyles returns the dotted dialect's house styles, resolved from
// the active theme.
func DefaultFactStyles() FactStyles {
	return FactStyles{
		Key:       lipgloss.NewStyle().Foreground(ColorTextDim()),
		SubKey:    lipgloss.NewStyle().Foreground(ColorTextFaint()),
		Value:     lipgloss.NewStyle(),
		Highlight: lipgloss.NewStyle().Foreground(ColorWarning()).Bold(true),
		Leader:    lipgloss.NewStyle().Foreground(ColorRule()),
	}
}

// FactLayout shapes one RenderFacts call: the leader dialect, the key/dot
// column width (0 means DefaultKeyColWidth for the column dialects), and the
// wrap budget (0 disables wrapping).
type FactLayout struct {
	Leader     FactLeader
	KeyWidth   int
	TotalWidth int
	Styles     FactStyles
}

// RenderFacts renders rows as styled fact lines in one of the three house
// dialects — the single engine behind the done cards' dotted leaders, the
// review sections' key columns, and the context pane's flowing facts. Lines
// come back un-downsampled.
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
		styled := keyStyle.Render(row.Key)
		valueStart = max(keyColWidth, lipgloss.Width(styled))
		prefix = styled + strings.Repeat(" ", valueStart-lipgloss.Width(styled))
	default: // FactLeaderDots
		keyLen := lipgloss.Width(row.Key)
		// Dots fill to the shared value column; a key close enough that fewer
		// than three dots remain keeps the column with what's left (floor 1),
		// and only a key overrunning the column itself shifts its own row —
		// sibling rows stay aligned.
		dotsNeeded := keyColWidth - keyLen - 2
		if dotsNeeded < 1 {
			dotsNeeded = 3
		}
		valueStart = keyLen + 1 + dotsNeeded + 1
		prefix = keyStyle.Render(row.Key) + " " + l.Styles.Leader.Render(strings.Repeat(".", dotsNeeded)) + " "
	}

	return strings.Split(prefix+wrapValueColumn(row.Value, valueStart, &valueStyle, l.TotalWidth), "\n")
}

// renderColonFact renders one "key: value" fact flow-wrapped to TotalWidth.
// The raw line is wrapped first and styled after (styling before wrapping
// would split ANSI sequences), so the key/value seam is re-found by rune
// count on each wrapped row.
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
