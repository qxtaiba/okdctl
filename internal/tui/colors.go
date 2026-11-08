package tui

import (
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
)

// ColorTheme selects between the default palette and a high-contrast variant.
type ColorTheme int

// Scaffolding: exported for a future 'okdctl theme' CLI verb; ResolveTheme is
// the only current consumer.
const (
	ThemeDefault ColorTheme = iota
	ThemeHighContrast
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

func highContrastRequested() bool {
	v := os.Getenv("OKDCTL_HIGH_CONTRAST")
	return v == "1" || v == "true"
}

// IsDarkBackground reports whether the terminal is currently treated as dark-background.
func IsDarkBackground() bool {
	return CurrentTheme().Dark
}

// SetDarkBackground re-resolves the active theme for a light- or dark-background terminal and rebuilds the base styles; the theme swap itself is atomic, but the style caches are not — call it before rendering starts.
func SetDarkBackground(dark bool) {
	resolveActiveTheme(dark)
}

func init() {
	resolveActiveTheme(true)
}
