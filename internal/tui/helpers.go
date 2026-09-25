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

type dottedKVOpts struct {
	highlight  bool
	subKey     bool
	totalWidth int
}

// wrapValueColumn renders value starting at column valueStart, wrapping it
// to fit totalWidth when value is too wide to fit and totalWidth > 0 (0
// disables wrapping), styling every line with valueStyle and right-padding
// each to totalWidth; a value that already fits is left byte-identical. A
// long key can leave less than minValueWrapWidth of room after valueStart —
// too narrow to wrap usefully — in which case wrapValueColumn falls back to
// a single line, rune-safe Truncate'd to that remaining room, the same
// rune-safe truncation the box itself uses, instead of wrapping to a budget
// wider than what's actually left.
func wrapValueColumn(value string, valueStart int, valueStyle *lipgloss.Style, totalWidth int) string {
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
		b.WriteString(valueStyle.Render(ln))
		if pad := totalWidth - valueStart - lipgloss.Width(ln); totalWidth > 0 && pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
	}
	return b.String()
}

func dottedKV(key, value string, keyColWidth int, opts dottedKVOpts) string {
	// Semantic theme roles, not raw palette tiers: the resolved Theme
	// carries the a11y and light-background variants to every rendered row.
	lines := RenderFacts(
		[]FactRow{{Key: key, Value: value, Highlight: opts.highlight, Sub: opts.subKey}},
		&FactLayout{Leader: FactLeaderDots, KeyWidth: keyColWidth, TotalWidth: opts.totalWidth, Styles: DefaultFactStyles()},
	)
	return Downsample(strings.Join(lines, "\n"))
}

// DottedKeyValueFull renders "key ....... value" padded to totalWidth, wrapping the value under the value column across several lines when it doesn't fit.
func DottedKeyValueFull(key, value string, keyColWidth, totalWidth int) string {
	return dottedKV(key, value, keyColWidth, dottedKVOpts{totalWidth: totalWidth})
}

// DottedKeyValueHighlightFull renders DottedKeyValueFull with an
// amber-highlighted value.
func DottedKeyValueHighlightFull(key, value string, keyColWidth, totalWidth int) string {
	return dottedKV(key, value, keyColWidth, dottedKVOpts{highlight: true, totalWidth: totalWidth})
}

// DottedKeyValueSubFull renders DottedKeyValueFull with a muted key for a nested row.
func DottedKeyValueSubFull(key, value string, keyColWidth, totalWidth int) string {
	return dottedKV(key, value, keyColWidth, dottedKVOpts{subKey: true, totalWidth: totalWidth})
}

// KeyValueNote renders "key    text" — key padded to keyColWidth with no dot leaders, wrapping text under the value column across several lines when it doesn't fit.
func KeyValueNote(key, text string, keyColWidth, totalWidth int) string {
	keyStyle := lipgloss.NewStyle().Foreground(ColorTextDim())
	valueStyle := lipgloss.NewStyle()

	if keyColWidth <= 0 {
		keyColWidth = DefaultKeyColWidth
	}

	keyLen := lipgloss.Width(key)
	pad := max(keyColWidth-keyLen, 1)
	valueStart := keyLen + pad

	prefix := keyStyle.Render(key) + strings.Repeat(" ", pad)
	return Downsample(prefix + wrapValueColumn(text, valueStart, &valueStyle, totalWidth))
}
