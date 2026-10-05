package wizard

import (
	"io"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// progressStep is a nop step that reports terminal progress, the seam the
// deploy stream drives OSC 9;4 through.
type progressStep struct {
	*nopStep
	state tea.ProgressBarState
	value int
}

func (s *progressStep) Update(tea.Msg) (WizardStep, tea.Cmd) { return s, nil }

func (s *progressStep) TerminalProgress() (state tea.ProgressBarState, value int) {
	return s.state, s.value
}

// recordingStep captures every message the frame delivers to it.
type recordingStep struct {
	*nopStep
	msgs []tea.Msg
}

func (s *recordingStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	s.msgs = append(s.msgs, msg)
	return s, nil
}

func TestViewRequestsFocusReporting(t *testing.T) {
	m := NewFlowModel([]WizardStep{newNopStep()}, config.DefaultConfig(), DefaultChrome())
	_ = tuitest.RenderAt(t, m, 100, 30)
	if !m.View().ReportFocus {
		t.Error("the wizard must ask the terminal for focus reports")
	}
}

// TestBlurDropsTheClockAndFocusRestoresIt pins away mode's clock contract:
// a blurred terminal runs the shared clock at 1Hz, and focus retires the
// pending slow tick so the catch-up sweep starts immediately.
func TestBlurDropsTheClockAndFocusRestoresIt(t *testing.T) {
	m := NewFlowModel([]WizardStep{newNopStep()}, config.DefaultConfig(), DefaultChrome())
	_ = tuitest.RenderAt(t, m, 100, 30)

	m.Update(tea.BlurMsg{})
	if !m.blurred {
		t.Fatal("BlurMsg must mark the model blurred")
	}
	if got := m.clockPeriod(); got != blurredFramePeriod {
		t.Errorf("blurred clock period = %v, want %v", got, blurredFramePeriod)
	}

	m.clockRunning = true
	gen := m.clockGen
	m.Update(tea.FocusMsg{})
	if m.blurred {
		t.Fatal("FocusMsg must clear the blur")
	}
	if got := m.clockPeriod(); got != framePeriod {
		t.Errorf("focused clock period = %v, want %v", got, framePeriod)
	}
	if m.clockGen == gen {
		t.Error("focus must retire the pending slow tick so the fast chain re-arms now")
	}
}

// TestFocusMsgsReachTheActiveStep keeps the step's own catch-up bookkeeping
// wired: blur and focus are delivered like any other message.
func TestFocusMsgsReachTheActiveStep(t *testing.T) {
	step := &recordingStep{nopStep: newNopStep()}
	m := NewFlowModel([]WizardStep{step}, config.DefaultConfig(), DefaultChrome())
	_ = tuitest.RenderAt(t, m, 100, 30)

	m.Update(tea.BlurMsg{})
	m.Update(tea.FocusMsg{})
	saw := map[string]bool{}
	for _, msg := range step.msgs {
		switch msg.(type) {
		case tea.BlurMsg:
			saw["blur"] = true
		case tea.FocusMsg:
			saw["focus"] = true
		}
	}
	if !saw["blur"] || !saw["focus"] {
		t.Errorf("step saw blur=%v focus=%v, want both delivered", saw["blur"], saw["focus"])
	}
}

// TestTerminalProgressGatedByColorProfile pins OSC 9;4's plain degradation:
// off-TTY (the test default) the view carries no progress bar at all, and a
// color-forced run carries the step's reading.
func TestTerminalProgressGatedByColorProfile(t *testing.T) {
	step := &progressStep{nopStep: newNopStep(), state: tea.ProgressBarDefault, value: 42}
	m := NewFlowModel([]WizardStep{step}, config.DefaultConfig(), DefaultChrome())
	_ = tuitest.RenderAt(t, m, 100, 30)

	if pb := m.View().ProgressBar; pb != nil {
		t.Errorf("off-TTY view must carry no progress bar, got %+v", pb)
	}

	t.Setenv("CLICOLOR_FORCE", "1")
	tui.SetColorProfileFor(io.Discard)
	t.Cleanup(func() {
		t.Setenv("CLICOLOR_FORCE", "")
		tui.SetColorProfileFor(io.Discard)
	})

	pb := m.View().ProgressBar
	if pb == nil || pb.State != tea.ProgressBarDefault || pb.Value != 42 {
		t.Fatalf("color-enabled view must carry the step's progress, got %+v", pb)
	}

	step.state = tea.ProgressBarNone
	if pb := m.View().ProgressBar; pb != nil {
		t.Errorf("a step reporting none must clear the terminal progress, got %+v", pb)
	}
}
