package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func newSpanSelector() *Selector {
	return NewSelector([]Option{
		{ID: "minor:4.20", Title: "okd 4.20", Description: "latest stable"},
		{ID: "4.20.1", Title: "  4.20.1", Description: "released: Aug 2026", InDropdown: true},
		{ID: "4.20.0", Title: "  4.20.0", Description: "released: Jun 2026", InDropdown: true},
		{ID: "minor:4.19", Title: "okd 4.19", Description: "stable"},
	})
}

func TestSelector_SelectedSpanTopLevel(t *testing.T) {
	s := newSpanSelector()
	lines := strings.Split(s.View(), "\n")

	start, end, ok := s.SelectedSpan()
	if !ok {
		t.Fatal("SelectedSpan() reported no span")
	}
	block := strings.Join(lines[start:end+1], "\n")
	if !strings.Contains(block, "okd 4.20") {
		t.Fatalf("span [%d,%d] = %q, want the okd 4.20 option", start, end, block)
	}
	if strings.Contains(block, "okd 4.19") {
		t.Fatalf("span [%d,%d] leaks the next option: %q", start, end, block)
	}
}

func TestSelector_SelectedSpanInsideDropdown(t *testing.T) {
	s := newSpanSelector()
	s.SetSelectedByID("4.20.0")
	lines := strings.Split(s.View(), "\n")

	start, end, ok := s.SelectedSpan()
	if !ok {
		t.Fatal("SelectedSpan() reported no span for a dropdown option")
	}
	if start < 0 || end >= len(lines) {
		t.Fatalf("span [%d,%d] is outside the %d rendered rows", start, end, len(lines))
	}
	block := strings.Join(lines[start:end+1], "\n")
	if !strings.Contains(block, "4.20.0") {
		t.Fatalf("span [%d,%d] = %q, want the 4.20.0 patch", start, end, block)
	}
	if strings.Contains(block, "4.20.1") {
		t.Fatalf("span [%d,%d] leaks the sibling patch: %q", start, end, block)
	}
}

func TestSelector_SelectedSpanEmptyOptions(t *testing.T) {
	s := NewSelector(nil)
	_ = s.View()
	if _, _, ok := s.SelectedSpan(); ok {
		t.Fatal("SelectedSpan() reported a span for an empty selector")
	}
}

func TestSelector_UpFromFirstDropdownItemReturnsToParent(t *testing.T) {
	s := newSpanSelector()
	s.SetSelectedByID("4.20.1")

	s.moveUp()

	if got := s.Selected().ID; got != "minor:4.20" {
		t.Fatalf("Selected().ID after moveUp() from the first dropdown item = %q, want minor:4.20 (the parent)", got)
	}
}

func TestSelector_DownFromLastDropdownItemContinues(t *testing.T) {
	s := newSpanSelector()
	s.SetSelectedByID("4.20.0")

	s.moveDown()

	if got := s.Selected().ID; got != "minor:4.19" {
		t.Fatalf("Selected().ID after moveDown() from the last dropdown item = %q, want minor:4.19 (the next parent)", got)
	}
}

func TestCompactSelector_ViewInlineRendersRowWithSelection(t *testing.T) {
	s := NewCompactSelector([]string{"a", "b", "c"})

	if got := tuitest.StripANSI(s.ViewInline()); got != "● a   ○ b   ○ c" {
		t.Fatalf("ViewInline() = %q, want %q", got, "● a   ○ b   ○ c")
	}

	s.selected = 1

	if got := tuitest.StripANSI(s.ViewInline()); got != "○ a   ● b   ○ c" {
		t.Fatalf("ViewInline() after selecting index 1 = %q, want %q", got, "○ a   ● b   ○ c")
	}
}

func TestArrowsAsVertical_MapsLeftRightToUpDown(t *testing.T) {
	if got := ArrowsAsVertical(tea.KeyPressMsg{Code: tea.KeyLeft}); got.Code != tea.KeyUp {
		t.Errorf("ArrowsAsVertical(left).Code = %v, want KeyUp", got.Code)
	}
	if got := ArrowsAsVertical(tea.KeyPressMsg{Code: tea.KeyRight}); got.Code != tea.KeyDown {
		t.Errorf("ArrowsAsVertical(right).Code = %v, want KeyDown", got.Code)
	}
	if got := ArrowsAsVertical(tea.KeyPressMsg{Code: tea.KeyEnter}); got.Code != tea.KeyEnter {
		t.Errorf("ArrowsAsVertical(enter).Code = %v, want unchanged KeyEnter", got.Code)
	}
	if got := ArrowsAsVertical(tea.KeyPressMsg{Code: 'j', Text: "j"}); got.Code != 'j' || got.Text != "j" {
		t.Errorf("ArrowsAsVertical('j') = %+v, want unchanged", got)
	}
}
