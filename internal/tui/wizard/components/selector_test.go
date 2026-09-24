package components

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

var nodeIDPattern = regexp.MustCompile(`node\d+`)

// shownNodeIDs returns the set of distinct "nodeN" IDs present in a
// rendered view, whole-token matched so "node1" never false-positives
// against a rendered "node10"..."node19".
func shownNodeIDs(view string) map[string]bool {
	shown := make(map[string]bool)
	for _, id := range nodeIDPattern.FindAllString(view, -1) {
		shown[id] = true
	}
	return shown
}

func sixItemDropdownSelector() *Selector {
	return nItemDropdownSelector(6)
}

func nItemDropdownSelector(n int) *Selector {
	opts := make([]Option, n)
	for i := range opts {
		opts[i] = Option{ID: fmt.Sprintf("node%d", i), Title: fmt.Sprintf("node%d", i), InDropdown: true}
	}
	return NewSelector(opts)
}

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

func TestSelector_DropdownBudgetShowsAllItemsWhenItFits(t *testing.T) {
	s := sixItemDropdownSelector()
	s.SetDropdownBudget(12)

	view := tuitest.StripANSI(s.View())
	for i := 0; i < 6; i++ {
		want := fmt.Sprintf("node%d", i)
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %s with budget 12:\n%s", want, view)
		}
	}
	if strings.Contains(view, "more") {
		t.Fatalf("view shows a more-marker despite the budget covering all items:\n%s", view)
	}
}

func TestSelector_DropdownBudgetClampsToFloor(t *testing.T) {
	s := sixItemDropdownSelector()
	s.SetDropdownBudget(3)

	view := tuitest.StripANSI(s.View())
	if !strings.Contains(view, "↓ 1 more") {
		t.Fatalf("budget 3 should clamp to the floor of 5, hiding exactly 1 of 6 items:\n%s", view)
	}
}

func TestSelector_DropdownOffsetReclampsWhenBudgetGrows(t *testing.T) {
	s := sixItemDropdownSelector()
	s.SetDropdownBudget(5) // the floor: only 5 of 6 items fit

	for i := 0; i < 5; i++ {
		s.moveDown()
	}
	if s.dropdownScrollOffset == 0 {
		t.Fatal("scrolling to the last item under a 5-item budget should have moved the offset")
	}

	// Simulate a resize growing the budget without moving the cursor: the
	// stale offset must re-clamp on render, not strand the window mid-list.
	s.SetDropdownBudget(12)
	view := tuitest.StripANSI(s.View())
	for i := 0; i < 6; i++ {
		want := fmt.Sprintf("node%d", i)
		if !strings.Contains(view, want) {
			t.Fatalf("growing the budget must re-clamp the stale offset and show %s:\n%s", want, view)
		}
	}
	if strings.Contains(view, "more") {
		t.Fatalf("budget 12 covers all items; a stale offset must not still show a more-marker:\n%s", view)
	}
}

func TestSelector_DropdownOffsetReclampsWhenBudgetShrinks(t *testing.T) {
	s := nItemDropdownSelector(20)
	s.SetDropdownBudget(10) // large enough to still need scrolling, short of the floor's ceiling

	for i := 0; i < 19; i++ {
		s.moveDown()
	}
	if s.Selected().ID != "node19" {
		t.Fatalf("Selected().ID after 19 moveDown() calls = %q, want node19", s.Selected().ID)
	}

	// Simulate a resize shrinking the budget toward the floor without
	// moving the cursor: the window must still contain the selected item
	// and render a full floor-sized set of rows, not strand the cursor
	// outside a window sized for the old, larger budget.
	s.SetDropdownBudget(5)
	view := tuitest.StripANSI(s.View())
	shown := shownNodeIDs(view)
	if !shown["node19"] {
		t.Fatalf("shrinking the budget must keep the selected item node19 in view:\n%s", view)
	}
	if len(shown) != 5 {
		t.Fatalf("shrunk window shows %d items %v, want exactly 5 (the floor)", len(shown), shown)
	}
}

func TestSelector_DropdownBudgetUnsetDefaultsToFloor(t *testing.T) {
	s := sixItemDropdownSelector()

	view := tuitest.StripANSI(s.View())
	if !strings.Contains(view, "↓ 1 more") {
		t.Fatalf("unset budget should default to today's floor of 5, hiding exactly 1 of 6 items:\n%s", view)
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
