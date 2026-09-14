package steps

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// heroMinWidth is the narrowest width renderHero draws the block-letter hero at.
const heroMinWidth = 60

// heroGlyphs holds each OKDCTL letter's 5-row block-character bitmap, left
// to right; repeated row strings (a straight stroke drawn the same way on
// several rows or letters) are the bitmap, not stringly-typed data, so
// goconst is suppressed.
//
//nolint:goconst,nolintlint // block-character bitmap rows repeat by design
var heroGlyphs = [6][5]string{
	{" ██████  ", "██    ██ ", "██    ██ ", "██    ██ ", " ██████  "}, // O
	{"██   ██ ", "██  ██  ", "█████   ", "██  ██  ", "██   ██ "},      // K
	{"██████  ", "██   ██ ", "██   ██ ", "██   ██ ", "██████  "},      // D
	{" ██████ ", "██      ", "██      ", "██      ", " ██████ "},      // C
	{"████████ ", "   ██    ", "   ██    ", "   ██    ", "   ██    "}, // T
	{"██      ", "██      ", "██      ", "██      ", "███████ "},      // L
}

// renderHero renders the OKDCTL block-letter hero gradient-colored across its
// six letters, or the spaced LogoStyle wordmark below heroMinWidth or without color.
func renderHero(width int, colorOK bool) string {
	if width < heroMinWidth || !colorOK {
		return wizard.LogoStyle.Render("O K D C T L")
	}
	rows := make([]string, 5)
	for r := range 5 {
		var b strings.Builder
		for l := range heroGlyphs {
			b.WriteString(lipgloss.NewStyle().Foreground(tui.LogoGradient[l]).Render(heroGlyphs[l][r]))
		}
		rows[r] = b.String()
	}
	return strings.Join(rows, "\n")
}
