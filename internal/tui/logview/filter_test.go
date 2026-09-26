package logview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// mixedRing seeds 40 lines where every fourth mentions etcd and two carry a
// real severity, so a filter, the jump keys and the counts all have something
// honest to work on.
func mixedRing() *Ring {
	r := NewRing(DefaultCap)
	for i := range 40 {
		l := Line{At: logBase.Add(time.Duration(i) * time.Second), Level: "INFO", Text: fmt.Sprintf("deploy step %02d", i)}
		if i%4 == 0 {
			l.Text = fmt.Sprintf("etcd member %02d healthy", i)
		}
		switch i {
		case 5:
			l.Level, l.Text = "WARN", "etcd member 05 slow to respond"
		case 30:
			l.Level, l.Text = "ERROR", "terraform apply failed"
		}
		r.Append(l)
	}
	return r
}

func typeFilter(s *Surface, text string) {
	s.HandleKey(tea.KeyPressMsg{Code: KeyFilter, Text: "/"}, false)
	for _, r := range text {
		s.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)}, false)
	}
}

// TestFilterSelectsOnlyMatchingRowsWithoutTouchingTheRing is the contract's
// core: the window shows matches only, and the ring still holds every line.
func TestFilterSelectsOnlyMatchingRowsWithoutTouchingTheRing(t *testing.T) {
	r := mixedRing()
	s := &Surface{Src: r}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)

	out := tuitest.StripANSI(s.RenderPane(70, 8))
	for _, row := range strings.Split(out, "\n")[1:] {
		if !strings.Contains(row, "etcd") {
			t.Errorf("filtered window shows a non-matching row: %q", row)
		}
	}
	if lines, _ := r.Snapshot(); len(lines) != 40 {
		t.Errorf("ring holds %d lines after filtering, want all 40", len(lines))
	}
	if !s.Filtered() {
		t.Error("a committed pattern must report as filtered")
	}
}

// TestFilterChipCountsMatchesOutOfTheStream pins the persistent chip.
func TestFilterChipCountsMatchesOutOfTheStream(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "etcd")

	out := tuitest.StripANSI(s.RenderPane(70, 8))
	if !strings.Contains(out, "/etcd (11/40)") {
		t.Errorf("typing chip must count matches out of the stream:\n%s", out)
	}
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)
	if out := tuitest.StripANSI(s.RenderPane(70, 8)); !strings.Contains(out, "filter: etcd (11/40)") {
		t.Errorf("committed chip must name the pattern:\n%s", out)
	}
}

// TestFilterNegationInvertsTheSelection pins the "!" prefix.
func TestFilterNegationInvertsTheSelection(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "!etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)

	out := tuitest.StripANSI(s.RenderPane(70, 8))
	if !strings.Contains(out, "filter: !etcd (29/40)") {
		t.Errorf("negated chip must count the complement:\n%s", out)
	}
	for _, row := range strings.Split(out, "\n")[1:] {
		if strings.Contains(row, "etcd") {
			t.Errorf("negated filter shows a matching row: %q", row)
		}
	}
}

// TestFilterInputEditsAndCancels pins the input mode: backspace shortens,
// esc restores whatever was committed before it opened.
func TestFilterInputEditsAndCancels(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)

	typeFilter(s, "xyz")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace}, false)
	if out := tuitest.StripANSI(s.RenderPane(70, 8)); !strings.Contains(out, "/xy (0/40)") {
		t.Errorf("backspace must shorten the pattern:\n%s", out)
	}
	if !s.CancelFilter() {
		t.Fatal("CancelFilter must report closing an open input")
	}
	if out := tuitest.StripANSI(s.RenderPane(70, 8)); !strings.Contains(out, "filter: etcd (11/40)") {
		t.Errorf("esc must restore the committed pattern:\n%s", out)
	}
	if s.CancelFilter() {
		t.Error("CancelFilter must report false with no input open")
	}
}

// TestFilterCommittedEmptyPatternClearsTheFilter keeps "/" then enter as the
// way back to the whole stream.
func TestFilterCommittedEmptyPatternClearsTheFilter(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)
	typeFilter(s, "")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)

	if s.Filtered() {
		t.Error("an empty committed pattern must clear the filter")
	}
	if out := tuitest.StripANSI(s.RenderPane(70, 8)); strings.Contains(out, "filter:") {
		t.Errorf("cleared filter must drop its chip:\n%s", out)
	}
}

// TestFilterInputConsumesTheFrameKeys is what keeps the wizard's scroll and
// help keys out of a filter the operator is typing.
func TestFilterInputConsumesTheFrameKeys(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	s.HandleKey(tea.KeyPressMsg{Code: KeyFilter, Text: "/"}, false)
	if !s.Filtering() {
		t.Fatal("the filter key must open the input")
	}
	if !s.ConsumesPaging() {
		t.Error("an open filter input must hold pgup/pgdn still")
	}
	for _, r := range []rune{'j', 'k', 'G', '?', 'l', 'f'} {
		s.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)}, false)
	}
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)
	if s.Full() || s.Locked() {
		t.Error("keys typed into the filter must not reach the lock or full-screen toggles")
	}
	if out := tuitest.StripANSI(s.RenderPane(70, 8)); !strings.Contains(out, "filter: jkG?lf") {
		t.Errorf("every typed key must land in the pattern:\n%s", out)
	}
}

// TestFilterComposesWithThePagerContract pins the coordinate contract: the
// locked span is stated in matches while a filter is live, and clearing the
// filter leaves the window at the same absolute point.
func TestFilterComposesWithThePagerContract(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)

	s.RenderPane(70, 6)
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyPgUp}, true)
	if !s.Locked() {
		t.Fatal("paging up a filtered window must engage the lock")
	}
	out := tuitest.StripANSI(s.RenderPane(70, 6))
	if !strings.Contains(out, "match 2–6 of 11") {
		t.Errorf("a locked filtered window must count in matches:\n%s", out)
	}

	at := s.LockPoint()
	typeFilter(s, "")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)
	if s.LockPoint() != at {
		t.Errorf("clearing the filter moved the lock from %d to %d", at, s.LockPoint())
	}
	if out := tuitest.StripANSI(s.RenderPane(70, 6)); !strings.Contains(out, "of 40") {
		t.Errorf("an unfiltered locked window counts absolute lines:\n%s", out)
	}
}

// TestJumpStopsOnWarningsAndErrorsWithNoFilter is the minimap's companion:
// the lane shows an error is back there, and N walks to it.
func TestJumpStopsOnWarningsAndErrorsWithNoFilter(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	s.RenderPane(70, 6)

	s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"}, true)
	if got := s.LockPoint(); got != 31 {
		t.Fatalf("first N landed at %d, want the error at index 30", got)
	}
	if out := tuitest.StripANSI(s.RenderPane(70, 6)); !strings.Contains(out, "terraform apply failed") {
		t.Errorf("the jump target must be on screen:\n%s", out)
	}

	s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"}, true)
	if got := s.LockPoint(); got != 6 {
		t.Fatalf("second N landed at %d, want the warning at index 5", got)
	}
	s.HandleKey(tea.KeyPressMsg{Code: KeyNextMatch, Text: "n"}, true)
	if got := s.LockPoint(); got != 31 {
		t.Fatalf("n landed at %d, want back to the error at index 30", got)
	}
}

// TestJumpStepsMatchesWhileFilteringKeepsTheKeysOneVocabulary pins the other
// half of the jump contract.
func TestJumpStepsMatchesWhileFilteringKeepsTheKeysOneVocabulary(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, false)
	s.RenderPane(70, 6)

	s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"}, true)
	if got := s.LockPoint(); got != 37 {
		t.Fatalf("N under a filter landed at %d, want the etcd line at index 36", got)
	}
	if s.JumpHelp() != "next/prev match" {
		t.Errorf("jump help = %q, want the match wording under a filter", s.JumpHelp())
	}
}

// TestJumpWithNothingToFindLeavesTheWindowAlone keeps the keys honest.
func TestJumpWithNothingToFindLeavesTheWindowAlone(t *testing.T) {
	r := seededRing(20)
	s := &Surface{Src: r}
	s.RenderPane(70, 6)

	s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"}, true)
	if s.Locked() {
		t.Error("a stream with no warning or error must leave the window following")
	}
}
