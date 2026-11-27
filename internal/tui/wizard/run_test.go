package wizard

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestFlowResultReadsAnUnsetOutcomeAsCancelled(t *testing.T) {
	step := &fakeStep{id: StepIDBasics}
	m := NewModel([]WizardStep{step}, config.DefaultConfig())

	if got := flowResult(m, m.steps); got.Outcome != OutcomeCancelled || got.Config != nil {
		t.Fatalf("result of a flow that never completed = %+v, want cancelled with no config", got)
	}

	m.Update(StepCompleteMsg{StepID: StepIDBasics})
	if got := flowResult(m, m.steps); got.Outcome != OutcomeCompleted || got.Config == nil {
		t.Fatalf("result of a completed flow = %+v, want completed with the config", got)
	}
}
