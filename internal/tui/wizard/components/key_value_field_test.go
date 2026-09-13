package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestKeyValueField_CardWidthExact(t *testing.T) {
	f := NewKeyValueField("vaults")
	f.SetValue("homelab=1")
	f.SetWidth(40)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	for i, r := range rows {
		if got := lipgloss.Width(r); got != 40 {
			t.Fatalf("row %d width = %d, want 40: %q", i, got, r)
		}
	}
}

func TestKeyValueField_EditModeJoinsTwoBoxes(t *testing.T) {
	f := NewKeyValueField("vaults")
	f.SetValue("homelab=1")
	f.SetWidth(40)
	_ = f.Focus()
	f.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")

	top := -1
	for i, r := range rows {
		if strings.Contains(r, "╮  ╭") {
			top = i
			break
		}
	}
	if top < 0 {
		t.Fatalf("no row contains the joined box seam %q:\n%s", "╮  ╭", strings.Join(rows, "\n"))
	}
	if top+2 >= len(rows) {
		t.Fatalf("only %d rows follow the seam, want at least 3", len(rows)-top)
	}

	editRows := rows[top : top+3]
	w := lipgloss.Width(editRows[0])
	for _, r := range editRows {
		if got := lipgloss.Width(r); got != w {
			t.Fatalf("edit-mode rows have unequal width: %d vs %d (%q)", got, w, r)
		}
	}
	if !strings.Contains(editRows[2], "╯  ╰") {
		t.Fatalf("bottom row of joined boxes = %q, want to contain %q", editRows[2], "╯  ╰")
	}

	colW := f.cellWidth()
	joined := lipgloss.JoinHorizontal(lipgloss.Top,
		fieldBox("x", colW, true, false), "  ", fieldBox("y", colW, false, false))
	joinedWidth := lipgloss.Width(strings.Split(joined, "\n")[0])
	if inner := f.width - 2; joinedWidth > inner {
		t.Fatalf("joined edit row width %d exceeds card inner width %d", joinedWidth, inner)
	}
}

func TestKeyValueField_AddRowTrailer(t *testing.T) {
	f := NewKeyValueField("vaults")
	f.SetWidth(40)

	got := tuitest.StripANSI(f.View())
	if !strings.Contains(got, "+ add") {
		t.Fatalf("View() = %q, want to contain %q", got, "+ add")
	}

	_ = f.Focus()
	f.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if len(f.rows) != 2 {
		t.Fatalf("rows after 'a' = %d, want 2", len(f.rows))
	}
}

func TestKeyValueField_ValueRoundTrip(t *testing.T) {
	f := NewKeyValueField("vaults")
	f.SetWidth(40)
	f.SetValue("homelab=1,shared=2")

	if got := f.Value(); got != "homelab=1,shared=2" {
		t.Fatalf("Value() = %q, want %q", got, "homelab=1,shared=2")
	}
}
