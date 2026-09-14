package steps

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestHeroGlyphsAre5RowsBy50Cols(t *testing.T) {
	if len(heroGlyphs) != 6 {
		t.Fatalf("len(heroGlyphs) = %d, want 6", len(heroGlyphs))
	}
	for l := range heroGlyphs {
		if len(heroGlyphs[l]) != 5 {
			t.Fatalf("heroGlyphs[%d] has %d rows, want 5", l, len(heroGlyphs[l]))
		}
	}
	for r := range 5 {
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
		for r := 1; r < 5; r++ {
			if got := lipgloss.Width(heroGlyphs[l][r]); got != want {
				t.Errorf("heroGlyphs[%d] row %d is %d cols wide, want %d (row 0's width) — a per-letter width mismatch can hide behind a correct row sum", l, r, got, want)
			}
		}
	}
}

func TestHeroFallsBackBelow60Cols(t *testing.T) {
	want := wizard.LogoStyle.Render("O K D C T L")
	if got := renderHero(heroMinWidth-1, true); got != want {
		t.Errorf("renderHero(%d, true) = %q, want the spaced wordmark %q", heroMinWidth-1, got, want)
	}
}

func TestHeroFallsBackWithoutColor(t *testing.T) {
	want := wizard.LogoStyle.Render("O K D C T L")
	if got := renderHero(80, false); got != want {
		t.Errorf("renderHero(80, false) = %q, want the spaced wordmark %q", got, want)
	}
}
