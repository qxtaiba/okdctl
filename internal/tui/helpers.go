package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ═══════════════════════════════════════════════════════════════════════════════
// DOTTED KEY-VALUE FORMATTING
// ═══════════════════════════════════════════════════════════════════════════════

// Default key column width for dotted key-value pairs (dots end at this position)
const DefaultKeyColWidth = 28

// dottedKVOpts contains options for formatting dotted key-value pairs.
type dottedKVOpts struct {
	highlight  bool // use amber bold for value
	totalWidth int  // if > 0, pad to this width
}

// dottedKV is the core implementation for all dotted key-value formatting.
func dottedKV(key, value string, keyColWidth int, opts dottedKVOpts) string {
	keyStyle := lipgloss.NewStyle().Foreground(ColorSlate400)
	dotStyle := lipgloss.NewStyle().Foreground(ColorSlate700)

	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	if opts.highlight {
		valueStyle = lipgloss.NewStyle().Foreground(ColorAmber500).Bold(true)
	}

	if keyColWidth <= 0 {
		keyColWidth = DefaultKeyColWidth
	}

	// Calculate dots needed to reach the fixed value column
	keyLen := lipgloss.Width(key)
	dotsNeeded := keyColWidth - keyLen - 2 // -2 for spaces around dots
	if dotsNeeded < 3 {
		dotsNeeded = 3
	}

	result := keyStyle.Render(key) + " " +
		dotStyle.Render(strings.Repeat(".", dotsNeeded)) + " " +
		valueStyle.Render(value)

	// Add right padding if total width specified
	if opts.totalWidth > 0 {
		baseLen := keyLen + 1 + dotsNeeded + 1 + lipgloss.Width(value)
		if rightPad := opts.totalWidth - baseLen; rightPad > 0 {
			result += strings.Repeat(" ", rightPad)
		}
	}

	return result
}

// DottedKeyValueFull creates a key-value line that fills to a total width.
// This ensures lines align properly within boxed panels without trailing whitespace.
func DottedKeyValueFull(key, value string, keyColWidth, totalWidth int) string {
	return dottedKV(key, value, keyColWidth, dottedKVOpts{totalWidth: totalWidth})
}

// DottedKeyValueHighlightFull creates a key-value line with highlighted value that fills to total width.
func DottedKeyValueHighlightFull(key, value string, keyColWidth, totalWidth int) string {
	return dottedKV(key, value, keyColWidth, dottedKVOpts{highlight: true, totalWidth: totalWidth})
}
