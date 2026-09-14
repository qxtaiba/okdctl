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
	errStyle   = lipgloss.NewStyle().Foreground(tui.ColorError)

	// helpStyle and tagStyle are assigned by RebuildStyles since they
	// capture ColorSlate500, a tier SetDarkBackground rebinds.
	helpStyle lipgloss.Style
	tagStyle  lipgloss.Style
)

// stylesGeneration counts RebuildStyles calls; a per-instance style cache
// elsewhere in the package (e.g. Selector.cachedStyles) records the
// generation it was built at and rebuilds once this counter moves past it,
// instead of assuming tui.Color* never changes after init.
var stylesGeneration int

// RebuildStyles assigns helpStyle and tagStyle from the current
// tui.ColorSlate500 value and advances stylesGeneration so per-instance
// caches elsewhere in the package invalidate; call at init and whenever
// tui.SetDarkBackground rebinds a tier.
func RebuildStyles() {
	helpStyle = lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	tagStyle = lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	stylesGeneration++
}

func init() {
	RebuildStyles()
}
