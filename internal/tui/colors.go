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

// SuccessGradient returns n colors from the active success tier to the logo
// gradient's cyan end.
func SuccessGradient(n int) []color.Color {
	if n < 1 {
		return nil
	}
	from, to := ColorSuccess(), LogoGradient[len(LogoGradient)-1]
	if n == 1 {
		return []color.Color{from}
	}
	ramp := make([]color.Color, n)
	for i := range ramp {
		ramp[i] = BlendAt(from, to, float64(i)/float64(n-1))
	}
	return ramp
}

// BlendAt returns the color t of the way from a to b (t clamped to [0, 1]),
// interpolating in RGB space; the install instrument paints its gradient
// fill with it.
func BlendAt(a, b color.Color, t float64) color.Color {
	t = min(max(t, 0), 1)
	fromR, fromG, fromB, _ := a.RGBA()
	toR, toG, toB, _ := b.RGBA()
	lerp := func(x, y uint32) uint8 {
		return uint8(uint32(float64(x>>8) + (float64(y>>8)-float64(x>>8))*t)) //nolint:gosec // G115: 8-bit channel values
	}
	return color.RGBA{R: lerp(fromR, toR), G: lerp(fromG, toG), B: lerp(fromB, toB), A: 0xFF}
}

// Lighten returns c moved amount of the way toward white (amount clamped to
// [0, 1]); the progress bar's drifting highlight band renders with it.
func Lighten(c color.Color, amount float64) color.Color {
	return BlendAt(c, color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, amount)
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
