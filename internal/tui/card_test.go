package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestCard_TitleInBorder(t *testing.T) {
	card := Card("vaults", "one\ntwo", 40, ColorPrimary())
	rows := strings.Split(tuitest.StripANSI(card), "\n")

	if !strings.HasPrefix(rows[0], "╭─ vaults ─") {
		t.Fatalf("row 0 = %q, want prefix %q", rows[0], "╭─ vaults ─")
	}
	for i, r := range rows {
		if got := lipgloss.Width(r); got != 40 {
			t.Errorf("row %d width = %d, want 40: %q", i, got, r)
		}
	}
}

// TestCard_OverLongLineNeverWidensBorder pins the box-geometry invariant: a
// body line longer than the card's inner width must be truncated, never
// allowed to push the right border outward.
func TestCard_OverLongLineNeverWidensBorder(t *testing.T) {
	overLong := "this body line is far longer than the twenty column card width"
	card := Card("title", overLong, 20, ColorPrimary())
	rows := strings.Split(tuitest.StripANSI(card), "\n")

	for i, r := range rows {
		if got := lipgloss.Width(r); got != 20 {
			t.Fatalf("row %d width = %d, want 20 (border must not widen): %q", i, got, r)
		}
	}
	if strings.Contains(rows[1], overLong) {
		t.Fatalf("over-long line rendered verbatim instead of being truncated: %q", rows[1])
	}
}

// TestCard_OverLongStyledLineNeverWidensBorder pins the same invariant for a
// body line that already carries its own ANSI styling, mirroring how
// callers like the hub dashboard pre-style tile content before handing it
// to Card.
func TestCard_OverLongStyledLineNeverWidensBorder(t *testing.T) {
	styled := lipgloss.NewStyle().Bold(true).Foreground(ColorWarning()).
		Render("this styled body line is far longer than the card width")
	card := Card("title", styled, 20, ColorPrimary())
	rows := strings.Split(card, "\n")

	for i, r := range rows {
		if got := lipgloss.Width(r); got != 20 {
			t.Fatalf("row %d visible width = %d, want 20 (border must not widen): %q", i, got, tuitest.StripANSI(r))
		}
	}
}
