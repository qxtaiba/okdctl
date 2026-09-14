package components

import (
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// fieldBox renders content in a rounded-border box exactly outer columns
// wide; the border colors red on hasErr, purple on focused, and slate
// otherwise — an error border always wins over a focus border.
func fieldBox(content string, outer int, focused, hasErr bool) string {
	border := tui.ColorSlate600
	switch {
	case hasErr:
		border = tui.ColorError
	case focused:
		border = tui.ColorPrimary
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(outer).
		Render(content)
}

var (
	labelStyle = lipgloss.NewStyle().Foreground(tui.ColorSlate300)
	helpStyle  = lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	errStyle   = lipgloss.NewStyle().Foreground(tui.ColorError)
	tagStyle   = lipgloss.NewStyle().Foreground(tui.ColorSlate500)
)
