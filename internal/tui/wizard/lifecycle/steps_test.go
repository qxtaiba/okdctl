package lifecycle

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestNewStepsOrder(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig()}
	steps := NewSteps(st, Hooks{})
	want := []wizard.StepID{StepIDOp, StepIDTarget, StepIDParams, StepIDPreview, StepIDConfirm, StepIDExec, StepIDDone}
	if len(steps) != len(want) {
		t.Fatalf("len(steps) = %d, want %d", len(steps), len(want))
	}
	for i, s := range steps {
		if s.ID() != want[i] {
			t.Errorf("steps[%d] = %s, want %s", i, s.ID(), want[i])
		}
	}
}
