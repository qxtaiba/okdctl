package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestCard_TitleInBorder(t *testing.T) {
	card := Card("vaults", "one\ntwo", 40, ColorPrimary)
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
