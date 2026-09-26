package steps

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// heroCols is the OKDCTL wordmark's own width at scale 1.
var heroCols = tui.WordmarkWidth("OKDCTL", 1)

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
	if rows != tui.WordmarkRows*heroScale || cols != heroCols*heroScale {
		t.Errorf("renderHero(%d, %d, true) = %dx%d, want %dx%d",
			heroDoubleWidth, heroDoubleHeight, rows, cols, tui.WordmarkRows*heroScale, heroCols*heroScale)
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
		if rows != tui.WordmarkRows || cols != heroCols {
			t.Errorf("renderHero(%d, %d, true) = %dx%d, want the standard %dx%d", c.w, c.h, rows, cols, tui.WordmarkRows, heroCols)
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
}
