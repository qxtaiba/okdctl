package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

var productWordmarks = []string{"OKDCTL", "DEPLOYED"}

func TestWordmarkCoversEveryProductWordmark(t *testing.T) {
	for _, word := range productWordmarks {
		for _, letter := range word {
			if _, ok := wordmarkGlyphs[letter]; !ok {
				t.Errorf("no bitmap for %q, needed by %q", letter, word)
			}
		}
	}
}

func TestWordmarkGlyphsAreRectangular(t *testing.T) {
	for letter, glyph := range wordmarkGlyphs {
		want := lipgloss.Width(glyph[0])
		for r := 1; r < WordmarkRows; r++ {
			if got := lipgloss.Width(glyph[r]); got != want {
				t.Errorf("%q row %d is %d cols, want row 0's %d", letter, r, got, want)
			}
		}
	}
}

func TestWordmarkRendersToItsMeasuredSize(t *testing.T) {
	for _, word := range productWordmarks {
		for _, scale := range []int{1, 2} {
			rows := strings.Split(Wordmark(word, LogoGradient[:], scale), "\n")
			if len(rows) != WordmarkRows*scale {
				t.Errorf("%q at scale %d rendered %d rows, want %d", word, scale, len(rows), WordmarkRows*scale)
			}
			want := WordmarkWidth(word, scale)
			for i, row := range rows {
				if got := lipgloss.Width(row); got != want {
					t.Errorf("%q at scale %d row %d is %d cols, want %d", word, scale, i, got, want)
				}
			}
		}
	}
}

func TestWordmarkScaleRepeatsEveryCell(t *testing.T) {
	rows := strings.Split(Wordmark("OKDCTL", LogoGradient[:], 2), "\n")
	for i := 0; i < len(rows); i += 2 {
		if rows[i+1] != rows[i] {
			t.Fatalf("row %d differs from row %d; each glyph row must repeat", i+1, i)
		}
	}
	if got := stretchCells("█ ", 2); got != "██  " {
		t.Errorf("stretchCells(%q, 2) = %q, want %q", "█ ", got, "██  ")
	}
}

func TestWordmarkWithoutGradientRendersPlain(t *testing.T) {
	out := Wordmark("OKDCTL", nil, 1)
	if strings.Contains(out, "\x1b") {
		t.Errorf("an empty gradient must render uncolored: %q", out)
	}
}
