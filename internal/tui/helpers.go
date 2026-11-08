package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// DefaultKeyColWidth is the key-column width used when the caller doesn't
// override it.
const DefaultKeyColWidth = 24

// minValueWrapWidth floors the wrapped value column so a long key or a
// narrow totalWidth never collapses it to an unreadable sliver.
const minValueWrapWidth = 12

// wrapValueColumn renders value starting at column valueStart, wrapping it
// to fit totalWidth when value is too wide to fit and totalWidth > 0 (0
// disables wrapping), styling every line with valueStyle and right-padding
// each to totalWidth; a value that already fits is left byte-identical. A
// long key can leave less than minValueWrapWidth of room after valueStart —
// too narrow to wrap usefully — in which case wrapValueColumn falls back to
// a single line, rune-safe Truncate'd to that remaining room, the same
// rune-safe truncation the box itself uses, instead of wrapping to a budget
// wider than what's actually left.
func wrapValueColumn(value string, valueStart int, valueStyle *lipgloss.Style, totalWidth int, link string) string {
	lines := []string{value}
	if totalWidth > 0 {
		// One column stays reserved at the right edge — the same gutter
		// errorbox keeps — so a full-width value never renders flush
		// against the enclosing box border.
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

// DottedKeyValueFull renders "key ....... value" padded to totalWidth, wrapping the value under the value column across several lines when it doesn't fit.
func DottedKeyValueFull(key, value string, keyColWidth, totalWidth int) string {
	// Semantic theme roles, not raw palette tiers: the resolved Theme
	// carries the a11y and light-background variants to every rendered row.
	lines := RenderFacts(
		[]FactRow{{Key: key, Value: value}},
		&FactLayout{Leader: FactLeaderDots, KeyWidth: keyColWidth, TotalWidth: totalWidth, Styles: DefaultFactStyles()},
	)
	return Downsample(strings.Join(lines, "\n"))
}
