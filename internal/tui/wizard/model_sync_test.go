package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

type syncStep struct {
	id         StepID
	applyCalls int
}

func (s *syncStep) ID() StepID                           { return s.id }
func (s *syncStep) Title() string                        { return string(s.id) }
func (s *syncStep) Init() tea.Cmd                        { return nil }
func (s *syncStep) Update(tea.Msg) (WizardStep, tea.Cmd) { return s, nil }
func (s *syncStep) View(int, int) string                 { return "" }
func (s *syncStep) Apply(*config.Config) error           { s.applyCalls++; return nil }

func TestConfigSyncAppliesCurrentStep(t *testing.T) {
	step := &syncStep{id: StepIDBasics}
	model := NewFlowModel([]WizardStep{step}, config.DefaultConfig(), DefaultChrome())
	model.Update(ConfigSyncMsg{StepID: StepIDBasics})
	if step.applyCalls != 1 {
		t.Fatalf("apply calls=%d", step.applyCalls)
	}
}
