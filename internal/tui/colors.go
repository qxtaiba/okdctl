package tui

import (
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
)

// ColorTheme selects between the default palette and a high-contrast variant.
type ColorTheme int

// Scaffolding: exported for a future 'okdctl theme' CLI verb; setTheme is
// the only current caller.
const (
	ThemeDefault ColorTheme = iota
	ThemeHighContrast
)

// colorSlate500Hex is ColorSlate500's literal value, named so
// SetDarkBackground can reuse it instead of re-spelling the hex.
const colorSlate500Hex = "#64748B"

// Palette — literal hex values kept stable across themes; setTheme
// swaps the semantic aliases below.
var (
	ColorPurple600 = lipgloss.Color("#9333EA")
	ColorPurple800 = lipgloss.Color("#6B21A8")
	ColorPrimary   = ColorPurple600
	// ColorPrimaryDim tints box borders with the brand (not slate) so every
	// box reads as okdctl.
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
	ColorSlate500 = lipgloss.Color(colorSlate500Hex)
	ColorSlate600 = lipgloss.Color("#475569")
	ColorSlate700 = lipgloss.Color("#334155")
	ColorSlate900 = lipgloss.Color("#0F172A")

	ColorText    = ColorSlate100
	ColorTextDim = ColorSlate400
)

// LogoGradient is the six-color gradient painted left to right across the
// welcome hero's OKDCTL block letters.
var LogoGradient = [6]color.Color{
	lipgloss.Color("#C084FC"),
	lipgloss.Color("#A78BFA"),
	lipgloss.Color("#8B8CF6"),
	lipgloss.Color("#67A6F0"),
	lipgloss.Color("#4CC0E8"),
	lipgloss.Color("#22D3EE"),
}

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

func highContrastRequested() bool {
	v := os.Getenv("OKDCTL_HIGH_CONTRAST")
	return v == "1" || v == "true"
}

var darkBackground = true

// IsDarkBackground reports whether the terminal is currently treated as dark-background.
func IsDarkBackground() bool {
	return darkBackground
}

// SetDarkBackground rebinds the muted colour tiers for a light- or dark-background terminal and rebuilds the base styles — not safe for concurrent use, so call it once, before rendering starts.
func SetDarkBackground(dark bool) {
	darkBackground = dark
	if dark {
		ColorTextDim = ColorSlate400
		ColorSlate500 = lipgloss.Color(colorSlate500Hex)
		ColorSlate700 = lipgloss.Color("#334155")
	} else {
		ColorTextDim = ColorSlate600
		ColorSlate500 = lipgloss.Color(colorSlate500Hex)
		ColorSlate700 = ColorSlate300
	}
	rebuildStyles()
}

func init() {
	if highContrastRequested() {
		setTheme(ThemeHighContrast)
	}
	rebuildStyles()
}
