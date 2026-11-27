package steps

import (
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// Hero sizing gates: the block letters need heroMinWidth and double at
// heroDoubleWidth by heroDoubleHeight.
const (
	heroMinWidth     = 60
	heroDoubleWidth  = 120
	heroDoubleHeight = 34
	heroScale        = 2
)

// renderHero renders the gradient block-letter hero, doubled on a large
// terminal, or the spaced wordmark below heroMinWidth or without color.
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
