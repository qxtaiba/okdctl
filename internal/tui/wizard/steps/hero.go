package steps

import (
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// Hero sizing gates: heroMinWidth is the narrowest terminal renderHero draws
// the block-letter wordmark at, and heroDoubleWidth by heroDoubleHeight is the
// terminal it draws that wordmark at heroScale times glyph size on both axes.
const (
	heroMinWidth     = 60
	heroDoubleWidth  = 120
	heroDoubleHeight = 34
	heroScale        = 2
)

// renderHero renders the OKDCTL block-letter hero gradient-colored across its
// six letters — 5 rows by 50 cols normally, heroScale times that on both axes
// once the terminal reaches heroDoubleWidth by heroDoubleHeight — or the
// spaced LogoStyle wordmark below heroMinWidth or without color.
func renderHero(width, height int, colorOK bool) string {
	if width < heroMinWidth || !colorOK {
		return wizard.LogoStyle.Render("O K D C T L")
	}

	scale := 1
	if width >= heroDoubleWidth && height >= heroDoubleHeight {
		scale = heroScale
	}
	return tui.Wordmark("OKDCTL", tui.LogoGradient[:], scale)
}
