package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestMultiSelectField_ChipsWrapToWidth(t *testing.T) {
	options := []string{
		"network-interface-alpha",
		"network-interface-bravo",
		"network-interface-charlie",
		"network-interface-delta",
		"network-interface-echo",
		"network-interface-foxtrot",
		"network-interface-golf",
		"network-interface-hotel",
	}
	f := NewMultiSelectField("additional networks", options)
	f.SetWidth(40)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	boxRows := rows[1:]

	if !strings.HasPrefix(boxRows[0], "╭") {
		t.Fatalf("box top row = %q, want to start with ╭", boxRows[0])
	}
	if last := boxRows[len(boxRows)-1]; !strings.HasPrefix(last, "╰") {
		t.Fatalf("box bottom row = %q, want to start with ╰", last)
	}
	for _, r := range boxRows {
		if got := lipgloss.Width(r); got != 40 {
			t.Fatalf("row %q width = %d, want 40", r, got)
		}
	}

	contentRows := boxRows[1 : len(boxRows)-1]
	if len(contentRows) != len(options) {
		t.Fatalf("content rows = %d, want %d (one chip per row at this width)", len(contentRows), len(options))
	}
}

func TestMultiSelectField_CursorPrecedesChip(t *testing.T) {
	f := NewMultiSelectField("additional networks", []string{"vmbr0", "vmbr1"})
	f.SetWidth(40)
	f.SetValue("vmbr0")
	_ = f.Focus()

	got := tuitest.StripANSI(f.View())
	if !strings.Contains(got, "> [✓] vmbr0") {
		t.Fatalf("View() = %q, want to contain %q", got, "> [✓] vmbr0")
	}
	if !strings.Contains(got, "  [ ] vmbr1") {
		t.Fatalf("View() = %q, want the unfocused chip to have no cursor", got)
	}
}

func TestMultiSelectField_ValueRoundTrip(t *testing.T) {
	f := NewMultiSelectField("additional networks", []string{"vmbr0", "vmbr1", "vmbr2"})
	f.SetWidth(40)
	f.SetValue("vmbr0,vmbr2")

	if got := f.Value(); got != "vmbr0,vmbr2" {
		t.Fatalf("Value() = %q, want vmbr0,vmbr2", got)
	}

	_ = f.Focus()
	right := tea.KeyPressMsg{Code: tea.KeyRight}
	left := tea.KeyPressMsg{Code: tea.KeyLeft}
	space := tea.KeyPressMsg{Code: tea.KeySpace}

	f.Update(right) // cursor: vmbr0 -> vmbr1
	f.Update(space) // toggle vmbr1 on
	if got := f.Value(); got != "vmbr0,vmbr1,vmbr2" {
		t.Fatalf("Value() after right+space = %q, want vmbr0,vmbr1,vmbr2", got)
	}

	f.Update(left)  // cursor: vmbr1 -> vmbr0
	f.Update(space) // toggle vmbr0 off
	if got := f.Value(); got != "vmbr1,vmbr2" {
		t.Fatalf("Value() after left+space = %q, want vmbr1,vmbr2", got)
	}
}

func TestMultiSelectField_OversizedChipIsEllipsizedNotSplit(t *testing.T) {
	f := NewMultiSelectField("additional networks", []string{"extremely-long-bridge-interface-name-that-overflows-the-box"})
	f.SetWidth(20)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	boxRows := rows[1:]

	if !strings.HasPrefix(boxRows[0], "╭") {
		t.Fatalf("box top row = %q, want to start with ╭", boxRows[0])
	}
	last := boxRows[len(boxRows)-1]
	if !strings.HasPrefix(last, "╰") {
		t.Fatalf("box bottom row = %q, want to start with ╰", last)
	}
	for _, r := range boxRows {
		if got := lipgloss.Width(r); got != 20 {
			t.Fatalf("row %q width = %d, want 20", r, got)
		}
	}

	contentRows := boxRows[1 : len(boxRows)-1]
	if len(contentRows) != 1 {
		t.Fatalf("content rows = %d, want 1 (an oversized chip must not split across rows)", len(contentRows))
	}
	if !strings.Contains(contentRows[0], "[ ]") || !strings.Contains(contentRows[0], "…") {
		t.Fatalf("content row = %q, want the checkbox and an ellipsized name on the same row", contentRows[0])
	}
}
