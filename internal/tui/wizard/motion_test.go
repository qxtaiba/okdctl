package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
)

type animatorStep struct {
	nopStep
	animating bool
	frame     uint64
}

func (s *animatorStep) Animating() bool { return s.animating }

func (s *animatorStep) Update(msg tea.Msg) (WizardStep, tea.Cmd) {
	if f, ok := msg.(FrameMsg); ok {
		s.frame = f.Frame
	}
	return s, nil
}

func newAnimatorStep() *animatorStep {
	return &animatorStep{nopStep: *newNopStep(), animating: true}
}

func TestClockArmsOnlyWhileStepAnimates(t *testing.T) {
	step := newAnimatorStep()
	m := NewModel([]WizardStep{step}, config.DefaultConfig())

	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !m.clockRunning {
		t.Fatal("clock not armed while the current step animates")
	}
	gen := m.clockGen

	step.animating = false
	m.Update(clockTickMsg{gen: gen})
	if m.clockRunning {
		t.Fatal("clock still armed after the step stopped animating")
	}

	step.animating = true
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !m.clockRunning {
		t.Fatal("clock did not re-arm when animation resumed")
	}
	if m.clockGen == gen {
		t.Fatal("re-arm must advance the generation so stale chain ticks drop")
	}
}

func TestClockNeverArmsForStillSteps(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.clockRunning {
		t.Fatal("clock armed for a step with no animators")
	}
}

func TestClockNeverArmsUnderMotionOff(t *testing.T) {
	step := newAnimatorStep()
	m := NewModel([]WizardStep{step}, config.DefaultConfig())
	m.motion = tui.MotionOff

	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.clockRunning {
		t.Fatal("clock armed under MotionOff")
	}
}

func TestClockTickAdvancesFrameAndDeliversToStep(t *testing.T) {
	step := newAnimatorStep()
	m := NewModel([]WizardStep{step}, config.DefaultConfig())

	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	gen := m.clockGen

	_, cmd := m.Update(clockTickMsg{gen: gen})
	if step.frame != 1 {
		t.Fatalf("step frame = %d after one tick, want 1", step.frame)
	}
	if cmd == nil {
		t.Fatal("an animating step's tick must re-arm the chain")
	}

	m.Update(clockTickMsg{gen: gen})
	if step.frame != 2 {
		t.Fatalf("step frame = %d after two ticks, want the monotonic counter", step.frame)
	}
}

func TestClockDropsStaleGenerationTicks(t *testing.T) {
	step := newAnimatorStep()
	m := NewModel([]WizardStep{step}, config.DefaultConfig())

	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

	m.Update(clockTickMsg{gen: m.clockGen - 1})
	if step.frame != 0 {
		t.Fatalf("stale tick reached the step: frame = %d", step.frame)
	}
}

func TestFrameMsgDrivesStepsDirectly(t *testing.T) {
	step := newAnimatorStep()
	m := NewModel([]WizardStep{step}, config.DefaultConfig())

	m.Update(FrameMsg{Frame: 42})
	if step.frame != 42 {
		t.Fatalf("injected FrameMsg not delivered: frame = %d, want 42", step.frame)
	}
}
