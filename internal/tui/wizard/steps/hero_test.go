package steps

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestHeroGlyphsAre5RowsBy50Cols(t *testing.T) {
	if len(heroGlyphs) != 6 {
		t.Fatalf("len(heroGlyphs) = %d, want 6", len(heroGlyphs))
	}
	for l := range heroGlyphs {
		if len(heroGlyphs[l]) != heroRows {
			t.Fatalf("heroGlyphs[%d] has %d rows, want %d", l, len(heroGlyphs[l]), heroRows)
		}
	}
	for r := range heroRows {
		width := 0
		for l := range heroGlyphs {
			width += lipgloss.Width(heroGlyphs[l][r])
		}
		if width != 50 {
			t.Errorf("hero row %d is %d cols wide, want 50", r, width)
		}
	}
	for l := range heroGlyphs {
		want := lipgloss.Width(heroGlyphs[l][0])
		for r := 1; r < heroRows; r++ {
			if got := lipgloss.Width(heroGlyphs[l][r]); got != want {
				t.Errorf("heroGlyphs[%d] row %d is %d cols wide, want %d (row 0's width) — a per-letter width mismatch can hide behind a correct row sum", l, r, got, want)
			}
		}
	}
}

func TestHeroFallsBackBelow60Cols(t *testing.T) {
	want := wizard.LogoStyle.Render("O K D C T L")
	if got := renderHero(heroMinWidth-1, 24, true); got != want {
		t.Errorf("renderHero(%d, 24, true) = %q, want the spaced wordmark %q", heroMinWidth-1, got, want)
	}
}

func TestHeroFallsBackWithoutColor(t *testing.T) {
	want := wizard.LogoStyle.Render("O K D C T L")
	if got := renderHero(80, 24, false); got != want {
		t.Errorf("renderHero(80, 24, false) = %q, want the spaced wordmark %q", got, want)
	}
}

// heroSize measures a rendered hero's row count and widest row.
func heroSize(t *testing.T, hero string) (rows, cols int) {
	t.Helper()
	lines := strings.Split(hero, "\n")
	for _, line := range lines {
		if w := lipgloss.Width(line); w > cols {
			cols = w
		}
	}
	return len(lines), cols
}

func TestHeroDoublesAt120x34(t *testing.T) {
	rows, cols := heroSize(t, renderHero(heroDoubleWidth, heroDoubleHeight, true))
	if rows != heroRows*heroScale || cols != 50*heroScale {
		t.Errorf("renderHero(%d, %d, true) = %dx%d, want %dx%d",
			heroDoubleWidth, heroDoubleHeight, rows, cols, heroRows*heroScale, 50*heroScale)
	}
}

func TestHeroStaysStandardBelowTheDoubleGate(t *testing.T) {
	cases := []struct{ w, h int }{
		{heroDoubleWidth - 1, heroDoubleHeight},
		{heroDoubleWidth, heroDoubleHeight - 1},
		{80, 24},
	}
	for _, c := range cases {
		rows, cols := heroSize(t, renderHero(c.w, c.h, true))
		if rows != heroRows || cols != 50 {
			t.Errorf("renderHero(%d, %d, true) = %dx%d, want the standard %dx50", c.w, c.h, rows, cols, heroRows)
		}
	}
}

func TestHeroDoubleScaleRepeatsEveryCell(t *testing.T) {
	doubled := renderHero(heroDoubleWidth, heroDoubleHeight, true)
	rows := strings.Split(doubled, "\n")
	for i := 0; i < len(rows); i += heroScale {
		for j := 1; j < heroScale; j++ {
			if rows[i+j] != rows[i] {
				t.Fatalf("double-scale row %d differs from row %d; each glyph row must repeat %d times", i+j, i, heroScale)
			}
		}
	}
	if got := stretchCells("█ ", heroScale); got != "██  " {
		t.Errorf("stretchCells(%q, %d) = %q, want %q", "█ ", heroScale, got, "██  ")
	}
}
