package logview

import (
	"fmt"
	"slices"
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
	s.HandleKey(tea.KeyPressMsg{Code: KeyFilter, Text: "/"})
	for _, r := range text {
		s.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// TestFilterSelectsOnlyMatchingRowsWithoutTouchingTheRing is the contract's
// core: the window shows matches only, and the ring still holds every line.
func TestFilterSelectsOnlyMatchingRowsWithoutTouchingTheRing(t *testing.T) {
	r := mixedRing()
	s := &Surface{Src: r}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 8))
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

	out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 8))
	if !strings.Contains(out, "/etcd (11/40)") {
		t.Errorf("typing chip must count matches out of the stream:\n%s", out)
	}
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 8)); !strings.Contains(out, "filter: etcd (11/40)") {
		t.Errorf("committed chip must name the pattern:\n%s", out)
	}
}

// TestFilterNegationInvertsTheSelection pins the "!" prefix.
func TestFilterNegationInvertsTheSelection(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "!etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 8))
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
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	typeFilter(s, "xyz")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 8)); !strings.Contains(out, "/xy (0/40)") {
		t.Errorf("backspace must shorten the pattern:\n%s", out)
	}
	if !s.CancelFilter() {
		t.Fatal("CancelFilter must report closing an open input")
	}
	if out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 8)); !strings.Contains(out, "filter: etcd (11/40)") {
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
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	typeFilter(s, "")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if s.Filtered() {
		t.Error("an empty committed pattern must clear the filter")
	}
	if out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 8)); strings.Contains(out, "filter:") {
		t.Errorf("cleared filter must drop its chip:\n%s", out)
	}
}

// TestFilterInputConsumesTheFrameKeys is what keeps the wizard's scroll and
// help keys out of a filter the operator is typing.
func TestFilterInputConsumesTheFrameKeys(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	s.HandleKey(tea.KeyPressMsg{Code: KeyFilter, Text: "/"})
	if !s.Filtering() {
		t.Fatal("the filter key must open the input")
	}
	if !s.ConsumesPaging() {
		t.Error("an open filter input must hold pgup/pgdn still")
	}
	for _, r := range []rune{'j', 'k', 'G', '?', 'l', 'f'} {
		s.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.Full() || s.Locked() {
		t.Error("keys typed into the filter must not reach the lock or full-screen toggles")
	}
	if out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 8)); !strings.Contains(out, "filter: jkG?lf") {
		t.Errorf("every typed key must land in the pattern:\n%s", out)
	}
}

// TestFilterComposesWithThePagerContract pins the coordinate contract: the
// locked span is stated in matches while a filter is live, and clearing the
// filter leaves the window at the same absolute point.
func TestFilterComposesWithThePagerContract(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	tailFrame(s.Src, s.v, 70, 6)
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if !s.Locked() {
		t.Fatal("paging up a filtered window must engage the lock")
	}
	out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 6))
	if !strings.Contains(out, "match 2–6 of 11") {
		t.Errorf("a locked filtered window must count in matches:\n%s", out)
	}

	at := s.LockPoint()
	typeFilter(s, "")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.LockPoint() != at {
		t.Errorf("clearing the filter moved the lock from %d to %d", at, s.LockPoint())
	}
	if out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 6)); !strings.Contains(out, "of 40") {
		t.Errorf("an unfiltered locked window counts absolute lines:\n%s", out)
	}
}

// TestJumpStopsOnWarningsAndErrorsWithNoFilter is the minimap's companion:
// the lane shows an error is back there, and N walks to it.
func TestJumpStopsOnWarningsAndErrorsWithNoFilter(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	tailFrame(s.Src, s.v, 70, 6)

	s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"})
	if got := s.LockPoint(); got != 31 {
		t.Fatalf("first N landed at %d, want the error at index 30", got)
	}
	if out := tuitest.StripANSI(tailFrame(s.Src, s.v, 70, 6)); !strings.Contains(out, "terraform apply failed") {
		t.Errorf("the jump target must be on screen:\n%s", out)
	}

	s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"})
	if got := s.LockPoint(); got != 6 {
		t.Fatalf("second N landed at %d, want the warning at index 5", got)
	}
	s.HandleKey(tea.KeyPressMsg{Code: KeyNextMatch, Text: "n"})
	if got := s.LockPoint(); got != 31 {
		t.Fatalf("n landed at %d, want back to the error at index 30", got)
	}
}

// TestJumpStepsMatchesWhileFilteringKeepsTheKeysOneVocabulary pins the other
// half of the jump contract.
func TestJumpStepsMatchesWhileFilteringKeepsTheKeysOneVocabulary(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	tailFrame(s.Src, s.v, 70, 6)

	s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"})
	if got := s.LockPoint(); got != 37 {
		t.Fatalf("N under a filter landed at %d, want the etcd line at index 36", got)
	}
	if s.JumpHelp() != "next/prev match" {
		t.Errorf("jump help = %q, want the match wording under a filter", s.JumpHelp())
	}
}

// TestHandleKeyReportsMovedOnLockAndJump is what an owning step's narrow
// tail needs to nudge the outer viewport toward it: lock and jump both move
// the window without toggling layout, so the step cannot know to re-scroll
// unless HandleKey says so apart from the layout-toggle report KeyFull uses.
func TestHandleKeyReportsMovedOnLockAndJump(t *testing.T) {
	s := &Surface{Src: mixedRing()}
	tailFrame(s.Src, s.v, 70, 6)

	if _, moved := s.HandleKey(tea.KeyPressMsg{Code: KeyLock, Text: "l"}); !moved {
		t.Error("locking the window must report moved")
	}
	if _, moved := s.HandleKey(tea.KeyPressMsg{Code: KeyLock, Text: "l"}); !moved {
		t.Error("releasing the lock must report moved too")
	}
	if _, moved := s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"}); !moved {
		t.Error("N must report moved")
	}
	if _, moved := s.HandleKey(tea.KeyPressMsg{Code: KeyNextMatch, Text: "n"}); !moved {
		t.Error("n must report moved")
	}
	if relayout, moved := s.HandleKey(tea.KeyPressMsg{Code: KeyFull, Text: "f"}); !relayout || moved {
		t.Errorf("f must report relayout alone, got relayout=%v moved=%v", relayout, moved)
	}
}

// shownTexts reports the Text of every line s's window currently shows at
// budget rows, bypassing rendering so a header count's honest update (the
// stream grew) cannot mask a window that quietly moved too.
func shownTexts(s *Surface, budget int) []string {
	lines, first := s.Src.Snapshot()
	st := s.v.filter.selectFrom(lines, first)
	w, _, _ := windowIn(&st, s.v, budget)
	texts := make([]string, len(w))
	for i := range w {
		texts[i] = w[i].Text
	}
	return texts
}

// TestLockedFilteredWindowIgnoresLinesThatArriveAfterTheLock pins the
// unexercised combination of a committed filter and a lock together: lockAt
// is an absolute raw-stream index (lockedAt reads it off the unfiltered
// snapshot), so it holds regardless of whether a filter is selecting a
// subsequence — a line arriving afterward, matching or not, must not move
// the window, exactly as an unfiltered lock already proves. The header's
// total legitimately grows (the stream did), so the window itself — not the
// rendered frame — is the thing to compare.
func TestLockedFilteredWindowIgnoresLinesThatArriveAfterTheLock(t *testing.T) {
	r := mixedRing()
	s := &Surface{Src: r}
	typeFilter(s, "etcd")
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	s.HandleKey(tea.KeyPressMsg{Code: KeyLock, Text: "l"})
	if !s.Locked() || !s.Filtered() {
		t.Fatal("setup: window must be both filtered and locked")
	}
	lockPoint := s.LockPoint()
	before := shownTexts(s, 8)

	r.Append(Line{At: logBase.Add(41 * time.Second), Level: "INFO", Text: "etcd member 41 healthy"})
	r.Append(Line{At: logBase.Add(42 * time.Second), Level: "INFO", Text: "deploy step 42"})

	if got := s.LockPoint(); got != lockPoint {
		t.Errorf("lockAt moved from %d to %d after new lines arrived", lockPoint, got)
	}
	if after := shownTexts(s, 8); !slices.Equal(after, before) {
		t.Errorf("a locked filtered window must not change when new lines arrive:\nbefore: %v\nafter:  %v", before, after)
	}
}

// TestJumpWithNothingToFindLeavesTheWindowAlone keeps the keys honest.
func TestJumpWithNothingToFindLeavesTheWindowAlone(t *testing.T) {
	r := seededRing(20)
	s := &Surface{Src: r}
	tailFrame(s.Src, s.v, 70, 6)

	s.HandleKey(tea.KeyPressMsg{Code: KeyPrevMatch, Text: "N"})
	if s.Locked() {
		t.Error("a stream with no warning or error must leave the window following")
	}
}
