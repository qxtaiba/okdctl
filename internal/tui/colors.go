package tui

import (
	"os"

	"charm.land/lipgloss/v2"
)

type ColorTheme int

const (
	ThemeDefault ColorTheme = iota
	ThemeHighContrast
)

var (
	ColorPurple600 = lipgloss.Color("#9333EA")
	ColorPrimary   = ColorPurple600

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

	ColorBorder  = ColorSlate700
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
	hcColorBorder  = lipgloss.Color("#888888")
)

func setTheme(theme ColorTheme) {
	switch theme {
	case ThemeHighContrast:
		ColorPrimary = hcColorPrimary
		ColorSuccess = hcColorSuccess
		ColorWarning = hcColorWarning
		ColorError = hcColorError
		ColorInfo = hcColorInfo
		ColorText = hcColorText
		ColorTextDim = hcColorTextDim
		ColorBorder = hcColorBorder
	default:
		ColorPrimary = ColorPurple600
		ColorSuccess = ColorGreen500
		ColorWarning = ColorAmber500
		ColorError = ColorRed500
		ColorInfo = ColorBlue500
		ColorText = ColorSlate100
		ColorTextDim = ColorSlate400
		ColorBorder = ColorSlate700
	}
}

func init() {
	if os.Getenv("HOMELAB_HIGH_CONTRAST") == "1" || os.Getenv("HOMELAB_HIGH_CONTRAST") == "true" {
		setTheme(ThemeHighContrast)
	}
}
