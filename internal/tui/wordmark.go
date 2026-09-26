package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// WordmarkRows is the height of every scale-1 wordmark.
const WordmarkRows = 5

//nolint:goconst,nolintlint // block-character bitmap rows repeat by design
var wordmarkGlyphs = map[rune][WordmarkRows]string{
	'O': {" ██████  ", "██    ██ ", "██    ██ ", "██    ██ ", " ██████  "},
	'K': {"██   ██ ", "██  ██  ", "█████   ", "██  ██  ", "██   ██ "},
	'D': {"██████  ", "██   ██ ", "██   ██ ", "██   ██ ", "██████  "},
	'C': {" ██████ ", "██      ", "██      ", "██      ", " ██████ "},
	'T': {"████████ ", "   ██    ", "   ██    ", "   ██    ", "   ██    "},
	'L': {"██      ", "██      ", "██      ", "██      ", "███████ "},
	'E': {"██████ ", "██     ", "█████  ", "██     ", "██████ "},
	'P': {"██████  ", "██   ██ ", "██████  ", "██      ", "██      "},
	'Y': {"██   ██ ", " ██ ██  ", "  ███   ", "   ██   ", "   ██   "},
}

// Wordmark renders supported letters as a scaled, gradient-colored bitmap.
// Unsupported runes are skipped, so callers must use letters with a bitmap.
func Wordmark(text string, gradient []color.Color, scale int) string {
	scale = max(scale, 1)
	letters := []rune(text)

	rows := make([]string, 0, WordmarkRows*scale)
	for r := range WordmarkRows {
		var b strings.Builder
		for i, letter := range letters {
			glyph, ok := wordmarkGlyphs[letter]
			if !ok {
				continue
			}
			cells := stretchCells(glyph[r], scale)
			if len(gradient) == 0 {
				b.WriteString(cells)
				continue
			}
			b.WriteString(lipgloss.NewStyle().Foreground(gradient[i%len(gradient)]).Render(cells))
		}
		for range scale {
			rows = append(rows, b.String())
		}
	}
	return strings.Join(rows, "\n")
}

// WordmarkWidth reports the rendered width of text at scale.
func WordmarkWidth(text string, scale int) int {
	scale = max(scale, 1)
	w := 0
	for _, letter := range text {
		if glyph, ok := wordmarkGlyphs[letter]; ok {
			w += lipgloss.Width(glyph[0]) * scale
		}
	}
	return w
}

// stretchCells repeats each glyph cell to scale the wordmark horizontally.
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
