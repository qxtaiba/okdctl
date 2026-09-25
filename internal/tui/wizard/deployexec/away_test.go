package deployexec

import (
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// TestBlurSuspendsThePulseAndFocusSweeps pins away mode on the instrument:
// blur freezes the cosmetic pulse and records where the bar stood; focus
// arms a frame-driven catch-up sweep from that point to the current target.
func TestBlurSuspendsThePulseAndFocusSweeps(t *testing.T) {
	st := streamState()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)
	s.applyEvent(&Event{StepID: st.Plan[0].ID})
	cur = cur.Add(30 * time.Second) // quiet: the pulse is the live animator

	s.Update(tea.BlurMsg{})
	if !s.blurred {
		t.Fatal("BlurMsg must mark the step blurred")
	}
	s.frame = 8
	frozen := s.runningGlyph()
	s.frame = 16
	if got := s.runningGlyph(); got != frozen {
		t.Errorf("a blurred pulse must freeze, moved %q → %q", frozen, got)
	}

	// Progress lands while away.
	s.applyEvent(&Event{StepID: st.Plan[0].ID, Done: true, Took: time.Second})
	s.applyEvent(&Event{StepID: st.Plan[1].ID})
	s.applyEvent(&Event{StepID: st.Plan[1].ID, Done: true, Took: time.Second})

	s.Update(tea.FocusMsg{})
	if s.blurred {
		t.Fatal("FocusMsg must clear the blur")
	}
	if !s.sweeping {
		t.Fatal("focus after away progress must arm the catch-up sweep")
	}
	if got := s.barFillFrac(); got >= s.barTarget() {
		t.Errorf("the sweep must start below the target, fill %v target %v", got, s.barTarget())
	}
	s.frame += 20
	if got := s.barFillFrac(); got < s.barTarget()-0.01 {
		t.Errorf("the sweep must reach the target within its frames, fill %v target %v", got, s.barTarget())
	}
}

// TestFocusWithoutAwayProgressArmsNoSweep keeps a plain alt-tab quiet: with
// the bar where the operator left it there is nothing to catch up on.
func TestFocusWithoutAwayProgressArmsNoSweep(t *testing.T) {
	st := streamState()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	s.Update(tea.BlurMsg{})
	s.Update(tea.FocusMsg{})
	if s.sweeping {
		t.Error("focus with no away progress must not animate the bar")
	}
}

// TestBellGatedByColorProfile pins the attention escape's plain
// degradation: off-TTY there is no command at all — the byte stream stays
// identical — and a color-forced run rings.
func TestBellGatedByColorProfile(t *testing.T) {
	s := NewStreamStep(streamState(), Hooks{})
	if s.bell() != nil {
		t.Error("off-TTY the bell must degrade to nothing")
	}

	t.Setenv("CLICOLOR_FORCE", "1")
	tui.SetColorProfileFor(io.Discard)
	t.Cleanup(func() {
		t.Setenv("CLICOLOR_FORCE", "")
		tui.SetColorProfileFor(io.Discard)
	})
	if s.bell() == nil {
		t.Error("a color-enabled terminal must get the bell")
	}
}

// TestStallWhileBlurredRingsOnce pins the attention contract: the first
// frame that crosses the stall threshold while blurred rings, later frames
// stay quiet, and returning output re-arms the detector.
func TestStallWhileBlurredRingsOnce(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	tui.SetColorProfileFor(io.Discard)
	t.Cleanup(func() {
		t.Setenv("CLICOLOR_FORCE", "")
		tui.SetColorProfileFor(io.Discard)
	})

	st := streamState()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)
	for _, m := range st.Plan[:5] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: time.Second})
	}
	s.applyEvent(&Event{StepID: install.StepWaitBootstrap})
	s.Update(tea.BlurMsg{})

	cur = cur.Add(2 * time.Minute)
	if _, cmd := s.Update(wizard.FrameMsg{Frame: 1}); cmd == nil {
		t.Fatal("crossing the stall threshold while blurred must ring the bell")
	}
	if _, cmd := s.Update(wizard.FrameMsg{Frame: 2}); cmd != nil {
		t.Error("a stall rings once, not per frame")
	}

	s.applyEvent(&Event{StepID: install.StepWaitBootstrap})
	cur = cur.Add(2 * time.Minute)
	if _, cmd := s.Update(wizard.FrameMsg{Frame: 3}); cmd == nil {
		t.Error("new output must re-arm the stall bell")
	}
}

// TestTerminalProgressFollowsTheProposal pins the OSC 9;4 states: weighted
// percent while running, indeterminate during the bootstrap wait, error on
// failure, cleared once the flow moves on.
func TestTerminalProgressFollowsTheProposal(t *testing.T) {
	st := streamState()
	st.History = seededHistory()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s := newSeededStreamStep(st, &cur)

	for _, m := range st.Plan[:2] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: time.Second})
	}
	if state, value := s.TerminalProgress(); state != tea.ProgressBarDefault || value != s.percent() {
		t.Errorf("running progress = (%v, %d), want (default, %d)", state, value, s.percent())
	}

	for _, m := range st.Plan[2:5] {
		s.applyEvent(&Event{StepID: m.ID})
		s.applyEvent(&Event{StepID: m.ID, Done: true, Took: time.Second})
	}
	s.applyEvent(&Event{StepID: install.StepWaitBootstrap})
	if state, _ := s.TerminalProgress(); state != tea.ProgressBarIndeterminate {
		t.Errorf("bootstrap wait progress state = %v, want indeterminate", state)
	}

	s.finished = true
	s.st.Result = errStreamFailed
	if state, _ := s.TerminalProgress(); state != tea.ProgressBarError {
		t.Errorf("failed run progress state = %v, want error", state)
	}

	s.st.Result = nil
	if state, value := s.TerminalProgress(); state != tea.ProgressBarDefault || value != 100 {
		t.Errorf("finished run progress = (%v, %d), want (default, 100)", state, value)
	}
}
