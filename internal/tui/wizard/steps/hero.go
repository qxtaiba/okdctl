package steps

import (
	"strings"

	"charm.land/lipgloss/v2"

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

// heroRows is the row count of every letter's bitmap in heroGlyphs.
const heroRows = 5

// heroGlyphs holds each OKDCTL letter's 5-row block-character bitmap, left
// to right; repeated row strings (a straight stroke drawn the same way on
// several rows or letters) are the bitmap, not stringly-typed data, so
// goconst is suppressed.
//
//nolint:goconst,nolintlint // block-character bitmap rows repeat by design
var heroGlyphs = [6][heroRows]string{
	{" ██████  ", "██    ██ ", "██    ██ ", "██    ██ ", " ██████  "}, // O
	{"██   ██ ", "██  ██  ", "█████   ", "██  ██  ", "██   ██ "},      // K
	{"██████  ", "██   ██ ", "██   ██ ", "██   ██ ", "██████  "},      // D
	{" ██████ ", "██      ", "██      ", "██      ", " ██████ "},      // C
	{"████████ ", "   ██    ", "   ██    ", "   ██    ", "   ██    "}, // T
	{"██      ", "██      ", "██      ", "██      ", "███████ "},      // L
}

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

	rows := make([]string, 0, heroRows*scale)
	for r := range heroRows {
		var b strings.Builder
		for l := range heroGlyphs {
			cells := stretchCells(heroGlyphs[l][r], scale)
			b.WriteString(lipgloss.NewStyle().Foreground(tui.LogoGradient[l]).Render(cells))
		}
		for range scale {
			rows = append(rows, b.String())
		}
	}
	return strings.Join(rows, "\n")
}

// stretchCells repeats every cell of one glyph bitmap row scale times — the
// horizontal half of the double-scale hero, whose vertical half repeats the
// row itself. Every bitmap rune is one column wide, so the row's rendered
// width scales exactly.
func stretchCells(row string, scale int) string {
	if scale == 1 {
		return row
	}
	var b strings.Builder
	b.Grow(len(row) * scale)
	for _, r := range row {
		for range scale {
			b.WriteRune(r)
		}
	}
	return b.String()
}
