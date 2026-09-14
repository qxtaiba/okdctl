package lifecycle

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestStagesCoverEveryStepExactlyOnce(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig()}
	steps := NewSteps(st, Hooks{})

	seen := map[wizard.StepID]int{}
	for _, stage := range Stages() {
		for _, id := range stage.Steps {
			seen[id]++
		}
	}

	if len(seen) != len(steps) {
		t.Fatalf("stages cover %d distinct steps, want %d", len(seen), len(steps))
	}
	for _, s := range steps {
		if n := seen[s.ID()]; n != 1 {
			t.Errorf("step %s covered by %d stages, want exactly 1", s.ID(), n)
		}
	}
}

type displayTitler interface {
	DisplayTitle() string
}

func TestEveryLifecycleStepHasDisplayTitle(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpRemove}
	steps := NewSteps(st, Hooks{})

	want := map[wizard.StepID]string{
		StepIDOp:      "choose an operation",
		StepIDTarget:  "worker to remove",
		StepIDParams:  "operation parameters",
		StepIDPreview: "review the plan",
		StepIDConfirm: "confirm removal",
		StepIDExec:    opProgressLabel(node.OpRemove),
		StepIDDone:    "done",
	}

	for _, s := range steps {
		dt, ok := s.(displayTitler)
		if !ok {
			t.Fatalf("step %s does not implement DisplayTitle", s.ID())
		}
		got := dt.DisplayTitle()
		if got == "" {
			got = s.Title()
		}
		if got != want[s.ID()] {
			t.Errorf("step %s DisplayTitle = %q, want %q", s.ID(), got, want[s.ID()])
		}
	}
}
