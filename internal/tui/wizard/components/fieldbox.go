package components

import (
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// fieldBox renders content in a rounded box exactly outer columns wide; a
// disabled border wins over error, which wins over focus.
func fieldBox(content string, outer int, focused, hasErr, disabled bool) string {
	border := tui.ColorSubtle()
	switch {
	case disabled:
		border = tui.ColorRule()
	case hasErr:
		border = tui.ColorError()
	case focused:
		border = tui.ColorPrimary()
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(outer).
		Render(content)
}

// defaultTagReserve is the " default" tag's width, reserved for a field's
// whole life once it has carried a default so the box never resizes.
const defaultTagReserve = 8

// DefaultTagReserve lets the wizard's field pairing reserve the same room in
// both halves of a pair.
const DefaultTagReserve = defaultTagReserve

// Assigned by RebuildStyles: every role rebinds on the background flip.
var (
	labelStyle lipgloss.Style
	errStyle   lipgloss.Style
	helpStyle  lipgloss.Style
	tagStyle   lipgloss.Style
)

// stylesGeneration lets per-instance caches such as Selector.cachedStyles
// rebuild after a theme swap.
var stylesGeneration int

// RebuildStyles assigns every themed style from the active Theme and
// advances stylesGeneration; call at init and after every theme swap.
func RebuildStyles() {
	labelStyle = lipgloss.NewStyle().Foreground(tui.ColorTextSoft())
	errStyle = lipgloss.NewStyle().Foreground(tui.ColorError())
	helpStyle = lipgloss.NewStyle().Foreground(tui.ColorTextFaint())
	tagStyle = lipgloss.NewStyle().Foreground(tui.ColorTextFaint())
	rebuildHelpOverlayStyles()
	stylesGeneration++
}

func init() {
	RebuildStyles()
}
