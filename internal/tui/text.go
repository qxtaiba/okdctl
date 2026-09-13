package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// WrapLines reformats text as space-joined words wrapped to width, returning
// one string per output line; it never returns an empty slice.
func WrapLines(text string, width int) []string {
	return strings.Split(lipgloss.Wrap(strings.Join(strings.Fields(text), " "), width, ""), "\n")
}
