package components

import (
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// fieldBox renders content in a rounded-border box exactly outer columns
// wide; the border colors red on hasErr, purple on focused, and slate
// otherwise — an error border always wins over a focus border, and a
// disabled field (its current value has no effect, e.g. a drain timeout
// while skip-drain is selected) always wins over both, since there is
// nothing left to flag or focus toward.
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

// defaultTagReserve is the on-screen width of the " default" tag (a
// leading space plus "default"'s 7 characters) that InputField and
// SelectField join beside their box. Both types reserve this much room out
// of their box's available width for the field's entire life once it has
// ever carried a default value — not just while the tag is still visible —
// so the box never resizes when the user's first edit drops the tag.
const defaultTagReserve = 8

// DefaultTagReserve mirrors defaultTagReserve for a caller outside the
// package that must reserve the same room — the wizard's field pairing
// keeps a paired column's box width even with its sibling's by reserving
// this much in both halves whenever either one carries a default tag.
const DefaultTagReserve = defaultTagReserve

// All four are assigned by RebuildStyles: under the dual-polarity Theme
// every role rebinds on the background flip, so an init-captured color
// would freeze its dark value.
var (
	labelStyle lipgloss.Style
	errStyle   lipgloss.Style
	helpStyle  lipgloss.Style
	tagStyle   lipgloss.Style
)

// stylesGeneration counts RebuildStyles calls; a per-instance style cache
// elsewhere in the package (e.g. Selector.cachedStyles) records the
// generation it was built at and rebuilds once this counter moves past it,
// instead of assuming tui.Color* never changes after init.
var stylesGeneration int

// RebuildStyles assigns every themed style in the package from the active
// resolved Theme and advances stylesGeneration so per-instance caches
// elsewhere in the package invalidate; call at init and whenever the theme
// swaps (the wizard's BackgroundColorMsg site).
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
