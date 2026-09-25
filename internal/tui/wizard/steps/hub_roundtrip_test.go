package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
)

// hubModel returns a wizard sitting on the hub with a loaded save slot, plus
// the hub step itself. The lifecycle package imports neither this one nor its
// steps, so the real day-2 flow can be assembled here.
func hubModel(t *testing.T, w, h int) (*wizard.Model, *WelcomeStep) {
	t.Helper()
	tui.SetTerminalWidth(w)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, w, h)
	seedHubSaveSlot(m)
	return m, m.CurrentStep().(*WelcomeStep)
}

// selectHubVerb walks the hub's pointer onto verb with real keypresses.
func selectHubVerb(t *testing.T, m *wizard.Model, hub *WelcomeStep, verb HubVerb) {
	t.Helper()
	for range len(hub.entries) {
		if hub.SelectedVerb() == verb {
			return
		}
		m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	t.Fatalf("verb %v is not on the hub's menu", verb)
}

func TestHubManageVerbRoundTripsThroughTheRealLifecycleFlow(t *testing.T) {
	const w, h = 100, 30
	m, hub := hubModel(t, w, h)

	builds := 0
	var opSteps []wizard.WizardStep
	hub.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		builds++
		st := &lifecycle.State{Cfg: m.Config()}
		opSteps = lifecycle.NewSteps(st, lifecycle.DemoHooks(0))
		return opSteps, lifecycle.Chrome(), nil
	}})

	selectHubVerb(t, m, hub, HubVerbManageNodes)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(resolveCmd(t, cmd))

	if got := m.CurrentStep().ID(); got != lifecycle.StepIDOp {
		t.Fatalf("current step after the manage verb = %q, want the lifecycle entry screen", got)
	}
	plain := tuitest.StripANSI(tuitest.RenderAt(t, m, w, h))
	for _, want := range []string{"cluster lifecycle", "resize nodes", "esc hub"} {
		if !strings.Contains(plain, want) {
			t.Errorf("the lifecycle entry screen is missing %q:\n%s", want, plain)
		}
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := m.CurrentStep().ID(); got != wizard.StepIDWelcome {
		t.Fatalf("esc from the lifecycle entry screen = %q, want the hub", got)
	}
	back := tuitest.StripANSI(tuitest.RenderAt(t, m, w, h))
	if !strings.Contains(back, "manage nodes") || strings.Contains(back, "opening") {
		t.Errorf("the hub must come back intact with its notice cleared:\n%s", back)
	}

	firstOp := opSteps[0]
	_, again := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(resolveCmd(t, again))

	if builds != 2 {
		t.Errorf("the lifecycle flow was built %d times across two entries, want 2", builds)
	}
	if opSteps[0] == firstOp {
		t.Error("re-entry reused the previous lifecycle step instances; a swapped-out flow's state must not bleed into the next entry")
	}
	if got := m.CurrentStep().ID(); got != lifecycle.StepIDOp {
		t.Errorf("re-entry landed on %q, want the lifecycle entry screen", got)
	}
}

func TestHubIgnoresASecondConfirmWhileOpening(t *testing.T) {
	const w, h = 100, 30
	m, hub := hubModel(t, w, h)

	builds := 0
	hub.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		builds++
		st := &lifecycle.State{Cfg: m.Config()}
		return lifecycle.NewSteps(st, lifecycle.DemoHooks(0)), lifecycle.Chrome(), nil
	}})

	selectHubVerb(t, m, hub, HubVerbManageNodes)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, first := m.Update(enter)
	_, second := m.Update(enter)

	// confirm() builds nothing itself — it hands back the command that will.
	// The second enter must hand back none at all, so no second flow is ever
	// assembled and no second swap is ever queued.
	if second != nil {
		t.Errorf("the second confirm produced a command (%T); it must be ignored while one flow is opening", second())
	}

	m.Update(resolveCmd(t, first))
	if builds != 1 {
		t.Errorf("the flow was built %d times for a double enter, want 1", builds)
	}
	if got := m.CurrentStep().ID(); got != lifecycle.StepIDOp {
		t.Fatalf("current step = %q, want the lifecycle entry screen", got)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := m.CurrentStep().ID(); got != wizard.StepIDWelcome {
		t.Fatalf("esc after a double enter = %q, want the hub — the operator must never be stranded", got)
	}

	_, again := m.Update(enter)
	m.Update(resolveCmd(t, again))
	if got := m.CurrentStep().ID(); got != lifecycle.StepIDOp {
		t.Errorf("re-entry after a double enter = %q, want the lifecycle entry screen", got)
	}
	if builds != 2 {
		t.Errorf("the flow was built %d times in total, want 2 (one per accepted entry)", builds)
	}
}

func TestHubEmptyFlowSurfacesAnErrorInsteadOfWedging(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	s.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return nil, wizard.FlowChrome{}, nil
	}})

	selectVerb(t, s, HubVerbManageNodes)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	failed, ok := cmd().(hubFlowFailedMsg)
	if !ok {
		t.Fatalf("a provider returning no screens produced %T, want hubFlowFailedMsg", cmd())
	}
	s.Update(failed)
	if s.opening != "" {
		t.Error("an empty flow must clear the opening notice, or every later confirm is refused by the guard")
	}
}
