package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const DefaultKeyColWidth = 28

type dottedKVOpts struct {
	highlight  bool
	totalWidth int
}

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

	keyLen := lipgloss.Width(key)
	dotsNeeded := keyColWidth - keyLen - 2 // -2 for spaces around dots
	if dotsNeeded < 3 {
		dotsNeeded = 3
	}

	result := keyStyle.Render(key) + " " +
		dotStyle.Render(strings.Repeat(".", dotsNeeded)) + " " +
		valueStyle.Render(value)

	if opts.totalWidth > 0 {
		baseLen := keyLen + 1 + dotsNeeded + 1 + lipgloss.Width(value)
		if rightPad := opts.totalWidth - baseLen; rightPad > 0 {
			result += strings.Repeat(" ", rightPad)
		}
	}

	return result
}

func DottedKeyValueFull(key, value string, keyColWidth, totalWidth int) string {
	return dottedKV(key, value, keyColWidth, dottedKVOpts{totalWidth: totalWidth})
}

func DottedKeyValueHighlightFull(key, value string, keyColWidth, totalWidth int) string {
	return dottedKV(key, value, keyColWidth, dottedKVOpts{highlight: true, totalWidth: totalWidth})
}
