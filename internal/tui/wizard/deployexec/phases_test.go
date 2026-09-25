package deployexec

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// registeredSteps lists every step the deploy engine registers, the same
// StepDefs-derived listing deploy's checklist and dry-run both read.
func registeredSteps(t *testing.T) []okd.DeployStep {
	t.Helper()
	p := okd.New(okd.WithProjectRoot(t.TempDir()), okd.WithLogger(logutil.NopLogger))
	return p.DeploySteps(config.DefaultConfig())
}

func TestDeployPhases_CoverEveryRegisteredStepExactlyOnce(t *testing.T) {
	steps := registeredSteps(t)

	seen := make(map[string]int, len(steps))
	for _, s := range steps {
		phase, ok := PhaseOf(s.ID)
		if !ok {
			t.Errorf("step %q belongs to no phase", s.ID)
			continue
		}
		if !validPhase(phase) {
			t.Errorf("step %q maps to unknown phase %q", s.ID, phase)
		}
		seen[string(s.ID)]++
	}

	for id, n := range seen {
		if n != 1 {
			t.Errorf("step %q appears %d times in the registry, want exactly 1", id, n)
		}
	}
	if len(stepPhases) != len(steps) {
		t.Errorf("stepPhases maps %d steps, the registry has %d", len(stepPhases), len(steps))
	}
}

// TestDeployPhases_GroupsAreContiguousInExecutionOrder proves the checklist can
// render the phases as ordered groups: a phase whose steps interleave with
// another's could not collapse to one row.
func TestDeployPhases_GroupsAreContiguousInExecutionOrder(t *testing.T) {
	order := make(map[Phase]int, len(PhaseOrder()))
	for i, p := range PhaseOrder() {
		order[p] = i
	}

	last := -1
	for _, s := range registeredSteps(t) {
		phase, ok := PhaseOf(s.ID)
		if !ok {
			continue
		}
		idx := order[phase]
		if idx < last {
			t.Fatalf("step %q in phase %q runs after phase index %d; groups must not interleave", s.ID, phase, last)
		}
		last = idx
	}
	if last != len(PhaseOrder())-1 {
		t.Errorf("registry ends in phase index %d, want the last phase %d", last, len(PhaseOrder())-1)
	}
}

func validPhase(p Phase) bool {
	for _, want := range PhaseOrder() {
		if p == want {
			return true
		}
	}
	return false
}
