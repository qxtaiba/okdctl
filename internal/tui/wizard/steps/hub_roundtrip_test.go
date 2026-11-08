package steps

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

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
