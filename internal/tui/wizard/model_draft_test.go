package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

type draftSyncStep struct {
	id         StepID
	applyCalls int
}

func (s *draftSyncStep) ID() StepID                           { return s.id }
func (s *draftSyncStep) Title() string                        { return string(s.id) }
func (s *draftSyncStep) Init() tea.Cmd                        { return nil }
func (s *draftSyncStep) Update(tea.Msg) (WizardStep, tea.Cmd) { return s, nil }
func (s *draftSyncStep) View(int, int) string                 { return "" }
func (s *draftSyncStep) Apply(*config.Config) error           { s.applyCalls++; return nil }

func TestConfigSyncSavesCurrentDraftState(t *testing.T) {
	step := &draftSyncStep{id: StepIDBasics}
	model := NewFlowModel([]WizardStep{step}, config.DefaultConfig(), DefaultChrome())
	called := false
	model.draftStateSaver = func(_ *config.Config, gotStep StepID, field string, history map[string][]string) error {
		called = true
		if gotStep != StepIDBasics || field != "cluster_name" {
			t.Fatalf("saved cursor = %s/%s", gotStep, field)
		}
		if history["basics/cluster_name"][0] != "cluster-a" {
			t.Fatalf("saved history = %v", history)
		}
		return nil
	}
	model.steps[0] = &draftSyncWithCursor{draftSyncStep: step}
	model.Update(ConfigSyncMsg{StepID: StepIDBasics})
	if !called || step.applyCalls != 1 {
		t.Fatalf("save called=%v, apply calls=%d", called, step.applyCalls)
	}
}

type draftSyncWithCursor struct{ *draftSyncStep }

func (*draftSyncWithCursor) DraftFieldKey() string { return "cluster_name" }
func (*draftSyncWithCursor) FieldHistory() map[string][]string {
	return map[string][]string{"basics/cluster_name": {"cluster-a"}}
}
