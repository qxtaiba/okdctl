package tui

import (
	"os"

	"charm.land/lipgloss/v2"
)

// ColorTheme selects between the default palette and a high-contrast variant.
type ColorTheme int

// Scaffolding: ThemeDefault and ThemeHighContrast are exported for the
// future 'okdctl theme' CLI verb that will let users pick a palette at
// runtime; setTheme is the only current caller.
const (
	ThemeDefault ColorTheme = iota
	ThemeHighContrast
)

// Palette — literal hex values kept stable across themes; setTheme
// swaps the semantic aliases below.
var (
	ColorPurple600 = lipgloss.Color("#9333EA")
	ColorPurple800 = lipgloss.Color("#6B21A8")
	ColorPrimary   = ColorPurple600
	// ColorPrimaryDim tints box borders with the brand instead of slate grey,
	// so every box reads as okdctl at a glance while the brighter ColorPrimary
	// carries the title.
	ColorPrimaryDim = ColorPurple800

	ColorGreen500 = lipgloss.Color("#22C55E")
	ColorSuccess  = ColorGreen500

	ColorAmber500 = lipgloss.Color("#F59E0B")
	ColorWarning  = ColorAmber500

	ColorRed500 = lipgloss.Color("#EF4444")
	ColorError  = ColorRed500

	ColorBlue500 = lipgloss.Color("#3B82F6")
	ColorInfo    = ColorBlue500

	ColorCyan400 = lipgloss.Color("#22D3EE")
	ColorCyan500 = lipgloss.Color("#06B6D4")

	ColorSlate100 = lipgloss.Color("#F1F5F9")
	ColorSlate300 = lipgloss.Color("#CBD5E1")
	ColorSlate400 = lipgloss.Color("#94A3B8")
	ColorSlate500 = lipgloss.Color("#64748B")
	ColorSlate600 = lipgloss.Color("#475569")
	ColorSlate700 = lipgloss.Color("#334155")
	ColorSlate900 = lipgloss.Color("#0F172A")

	ColorText    = ColorSlate100
	ColorTextDim = ColorSlate400
)

var (
	hcColorPrimary = lipgloss.Color("#FF00FF")
	hcColorSuccess = lipgloss.Color("#00FF00")
	hcColorWarning = lipgloss.Color("#FFFF00")
	hcColorError   = lipgloss.Color("#FF0000")
	hcColorInfo    = lipgloss.Color("#00FFFF")
	hcColorText    = lipgloss.Color("#FFFFFF")
	hcColorTextDim = lipgloss.Color("#AAAAAA")
)

func setTheme(theme ColorTheme) {
	switch theme {
	case ThemeHighContrast:
		ColorPrimary = hcColorPrimary
		ColorPrimaryDim = hcColorPrimary
		ColorSuccess = hcColorSuccess
		ColorWarning = hcColorWarning
		ColorError = hcColorError
		ColorInfo = hcColorInfo
		ColorText = hcColorText
		ColorTextDim = hcColorTextDim
	default:
		ColorPrimary = ColorPurple600
		ColorPrimaryDim = ColorPurple800
		ColorSuccess = ColorGreen500
		ColorWarning = ColorAmber500
		ColorError = ColorRed500
		ColorInfo = ColorBlue500
		ColorText = ColorSlate100
		ColorTextDim = ColorSlate400
	}
}

// highContrastRequested checks OKDCTL_HIGH_CONTRAST and the legacy
// HOMELAB_HIGH_CONTRAST (kept working for existing scripts/dotfiles).
func highContrastRequested() bool {
	for _, name := range []string{"OKDCTL_HIGH_CONTRAST", "HOMELAB_HIGH_CONTRAST"} {
		if v := os.Getenv(name); v == "1" || v == "true" {
			return true
		}
	}
	return false
}

func init() {
	if highContrastRequested() {
		setTheme(ThemeHighContrast)
	}
}
