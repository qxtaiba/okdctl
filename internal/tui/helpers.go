package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// DefaultKeyColWidth is the key-column width when the caller sets none.
const DefaultKeyColWidth = 24

// minValueWrapWidth keeps the wrapped value column readable under a long key.
const minValueWrapWidth = 12

// wrapValueColumn renders value from column valueStart, wrapping to
// totalWidth (0 disables); with less than minValueWrapWidth left it
// truncates to one line instead.
func wrapValueColumn(value string, valueStart int, valueStyle *lipgloss.Style, totalWidth int, link string) string {
	lines := []string{value}
	if totalWidth > 0 {
		// One column stays reserved at the right edge so a full-width value never
		// touches the box border.
		remaining := totalWidth - valueStart - 1
		switch {
		case remaining < minValueWrapWidth:
			lines = []string{Truncate(value, remaining)}
		case lipgloss.Width(value) > remaining:
			lines = WrapLines(value, remaining)
		}
	}

	var b strings.Builder
	for i, ln := range lines {
		if i > 0 {
			b.WriteString("\n" + strings.Repeat(" ", valueStart))
		}
		b.WriteString(Hyperlink(link, valueStyle.Render(ln)))
		if pad := totalWidth - valueStart - lipgloss.Width(ln); totalWidth > 0 && pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
	}
	return b.String()
}

// DottedKeyValueFull renders "key ....... value" padded to totalWidth,
// wrapping the value under its column when it does not fit.
func DottedKeyValueFull(key, value string, keyColWidth, totalWidth int) string {
	lines := RenderFacts(
		[]FactRow{{Key: key, Value: value}},
		&FactLayout{Leader: FactLeaderDots, KeyWidth: keyColWidth, TotalWidth: totalWidth, Styles: DefaultFactStyles()},
	)
	return Downsample(strings.Join(lines, "\n"))
}
