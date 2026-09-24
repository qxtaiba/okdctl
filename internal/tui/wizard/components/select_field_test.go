package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestSelectField_ArrowsShownWhenBlurred(t *testing.T) {
	f := NewSelectField("cpu type", []string{"host", "x86-64-v2", "kvm64"})
	f.SetWidth(90)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if !strings.Contains(rows[2], "◂") || !strings.Contains(rows[2], "▸") {
		t.Fatalf("blurred content row = %q, want cycle arrows", rows[2])
	}
}

func TestSelectField_BooleanRendersRadioPair(t *testing.T) {
	f := NewSelectField("enable numa", []string{"yes", "no"})
	f.SetWidth(90)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	want := "● yes  ○ no"
	if !strings.Contains(rows[2], want) {
		t.Fatalf("content row = %q, want to contain %q", rows[2], want)
	}

	_ = f.Focus()
	f.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	rows = strings.Split(tuitest.StripANSI(f.View()), "\n")
	want = "○ yes  ● no"
	if !strings.Contains(rows[2], want) {
		t.Fatalf("after left, content row = %q, want to contain %q", rows[2], want)
	}
}

func TestSelectField_BoxWidthFromWidestOption(t *testing.T) {
	f := NewSelectField("cpu type", []string{"host", "x86-64-v2", "kvm64"})
	f.SetWidth(90)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if got := lipgloss.Width(rows[1]); got != 17 {
		t.Fatalf("box width = %d, want 17: %q", got, rows[1])
	}
}

func TestSelectField_MinBoxWidth12(t *testing.T) {
	f := NewSelectField("mode", []string{"a", "b"})
	f.SetWidth(90)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if got := lipgloss.Width(rows[1]); got != 12 {
		t.Fatalf("box width = %d, want 12: %q", got, rows[1])
	}
}

func TestSelectField_BoxWidthUsesDisplayWidthNotBytes(t *testing.T) {
	f := NewSelectField("drain mode", []string{"skip drain — restart pods in place"})
	f.SetWidth(90)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if got, want := lipgloss.Width(rows[1]), 42; got != want {
		t.Fatalf("box width = %d, want %d (rune width, not utf-8 byte length): %q", got, want, rows[1])
	}
}

func TestSelectField_DefaultTagBesideBox(t *testing.T) {
	f := NewSelectField("cpu type", []string{"host", "kvm64"})
	f.SetDefault("host")
	f.SetWidth(90)

	got := tuitest.StripANSI(f.View())
	if !strings.Contains(got, "default") {
		t.Fatalf("View() = %q, want a default tag", got)
	}
	if strings.Contains(got, "(default)") {
		t.Fatalf("View() = %q, want no (default) suffix", got)
	}
}

func TestSelectField_NoteRendersVerbatim(t *testing.T) {
	f := NewSelectField("drain mode", []string{"graceful", "force"})
	f.SetWidth(90)
	f.Note = "skip-drain leaves workloads running during node removal"

	got := f.View()
	if !strings.HasSuffix(got, "\n"+f.Note) {
		t.Fatalf("View() = %q, want to end with note %q", got, f.Note)
	}
}

func TestSelectField_SingleOptionHidesArrows(t *testing.T) {
	f := NewSelectField("proxmox node for bootstrap vm", []string{"pve1"})
	f.SetWidth(90)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if strings.Contains(rows[2], "◂") || strings.Contains(rows[2], "▸") {
		t.Fatalf("content row = %q, want no cycle arrows for a single option", rows[2])
	}
	if !strings.Contains(rows[2], "pve1") {
		t.Fatalf("content row = %q, want the bare value %q", rows[2], "pve1")
	}
}

func TestSelectField_SingleOptionArrowKeepsDefaultTag(t *testing.T) {
	f := NewSelectField("proxmox node for bootstrap vm", []string{"pve1"})
	f.SetDefault("pve1")
	f.SetWidth(90)
	_ = f.Focus()

	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	if !f.isDefault {
		t.Fatal("isDefault after a single-option arrow = false, want true (the selection can't change)")
	}
}

func TestSelectField_TwoOptionArrowDropsDefaultTag(t *testing.T) {
	f := NewSelectField("cpu type", []string{"host", "kvm64"})
	f.SetDefault("host")
	f.SetWidth(90)
	_ = f.Focus()

	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	if f.isDefault {
		t.Fatal("isDefault after a two-option arrow = true, want false (the selection changed)")
	}
}

// TestSelectField_BlankOptionRendersHonestLabel pins the fcos-iso field's
// shape (a leading blank option meaning "let okdctl download it"): the
// blank value must render as a dim "none" between the cycle arrows rather
// than a bare double space that reads as a rendering glitch.
func TestSelectField_BlankOptionRendersHonestLabel(t *testing.T) {
	f := NewSelectField("fcos iso", []string{"", "local:iso/fedora-coreos.iso"})
	f.SetWidth(90)

	rows := strings.Split(tuitest.StripANSI(f.View()), "\n")
	if !strings.Contains(rows[2], "◂ none ▸") {
		t.Fatalf("content row = %q, want the blank option labeled %q", rows[2], "◂ none ▸")
	}
}
