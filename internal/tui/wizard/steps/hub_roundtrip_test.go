package steps

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
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

func TestHubEmptyFlowSurfacesAnErrorInsteadOfWedging(t *testing.T) {
	s := NewWelcomeStep()
	s.SetConfigExists(true)
	s.SetFlows(HubFlows{ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
		return nil, wizard.FlowChrome{}, nil
	}})

	selectVerb(t, s, HubVerbManageNodes)
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	failed, ok := resolveCmd(t, cmd).(hubFlowFailedMsg)
	if !ok {
		t.Fatalf("a provider returning no screens produced %T, want hubFlowFailedMsg", resolveCmd(t, cmd))
	}
	s.Update(failed)
	if s.opening != "" {
		t.Error("an empty flow must clear the opening notice, or every later confirm is refused by the guard")
	}
}
