// Package tui provides terminal user interface components.
// Follows the "less is more" principle - minimal colors, maximum clarity.
package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// ColorTheme represents a color theme for accessibility.
type ColorTheme int

const (
	// ThemeDefault uses the standard color palette.
	ThemeDefault ColorTheme = iota
	// ThemeHighContrast uses high-contrast colors for better visibility.
	ThemeHighContrast
)

// ═══════════════════════════════════════════════════════════════════════════════
// COLOR PALETTE - Minimal set of actually-used colors
// ═══════════════════════════════════════════════════════════════════════════════

var (
	// Brand - Purple
	ColorPurple600 = lipgloss.Color("#9333EA")
	ColorPrimary   = ColorPurple600

	// Success - Green
	ColorGreen500 = lipgloss.Color("#22C55E")
	ColorSuccess  = ColorGreen500

	// Warning - Amber
	ColorAmber500 = lipgloss.Color("#F59E0B")
	ColorWarning  = ColorAmber500

	// Error - Red
	ColorRed500 = lipgloss.Color("#EF4444")
	ColorError  = ColorRed500

	// Info - Blue
	ColorBlue500 = lipgloss.Color("#3B82F6")
	ColorInfo    = ColorBlue500

	// Highlights - Cyan
	ColorCyan400 = lipgloss.Color("#22D3EE")
	ColorCyan500 = lipgloss.Color("#06B6D4")

	// Neutral - Slate
	ColorSlate100 = lipgloss.Color("#F1F5F9")
	ColorSlate300 = lipgloss.Color("#CBD5E1")
	ColorSlate400 = lipgloss.Color("#94A3B8")
	ColorSlate500 = lipgloss.Color("#64748B")
	ColorSlate600 = lipgloss.Color("#475569")
	ColorSlate700 = lipgloss.Color("#334155")
	ColorSlate900 = lipgloss.Color("#0F172A")

	// Semantic aliases
	ColorBorder  = ColorSlate700
	ColorText    = ColorSlate100
	ColorTextDim = ColorSlate400
)

// ═══════════════════════════════════════════════════════════════════════════════
// HIGH CONTRAST THEME - For accessibility
// ═══════════════════════════════════════════════════════════════════════════════

var (
	// High contrast colors use more saturated, higher contrast values.
	hcColorPrimary = lipgloss.Color("#FF00FF") // Bright magenta
	hcColorSuccess = lipgloss.Color("#00FF00") // Bright green
	hcColorWarning = lipgloss.Color("#FFFF00") // Bright yellow
	hcColorError   = lipgloss.Color("#FF0000") // Bright red
	hcColorInfo    = lipgloss.Color("#00FFFF") // Bright cyan
	hcColorText    = lipgloss.Color("#FFFFFF") // White
	hcColorTextDim = lipgloss.Color("#AAAAAA") // Light gray
	hcColorBorder  = lipgloss.Color("#888888") // Medium gray
)

// SetTheme sets the active color theme.
func SetTheme(theme ColorTheme) {
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
	// Check environment variable for high contrast mode
	if os.Getenv("HOMELAB_HIGH_CONTRAST") == "1" || os.Getenv("HOMELAB_HIGH_CONTRAST") == "true" {
		SetTheme(ThemeHighContrast)
	}
}
