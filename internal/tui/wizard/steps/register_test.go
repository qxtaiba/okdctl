package steps

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestDeployPhases_CoverEveryRegisteredStepExactlyOnce(t *testing.T) {
	builder := wizard.NewStepBuilder()
	RegisterAll(builder)
	built := wizard.BuildSteps(wizard.DefaultConfig(), builder)

	counts := make(map[wizard.StepID]int)
	for _, stage := range deployPhases() {
		for _, id := range stage.Steps {
			counts[id]++
		}
	}

	for _, step := range built.Steps {
		if counts[step.ID()] != 1 {
			t.Errorf("step %q appears in %d phases, want exactly 1", step.ID(), counts[step.ID()])
		}
	}
	if len(counts) != len(built.Steps) {
		t.Errorf("deployPhases lists %d step ids, RegisterAll has %d", len(counts), len(built.Steps))
	}
}

func TestRegisterAllSharesCapacitySnapshotWithReviewState(t *testing.T) {
	builder := wizard.NewStepBuilder()
	RegisterAll(builder)
	built := wizard.BuildSteps(wizard.DefaultConfig(), builder)
	capacity, ok := built.States[wizard.StepTypeReview].(*WizardCapacitySnapshot)
	if !ok || capacity == nil {
		t.Fatalf("review state = %T, want *WizardCapacitySnapshot", built.States[wizard.StepTypeReview])
	}
	resources, ok := built.States[wizard.StepTypeResources].(*ResourcesStepState)
	if !ok || resources.Capacity != capacity {
		t.Fatalf("resource snapshot = %p, review snapshot = %p", resources.Capacity, capacity)
	}
}

// TestPhaseTrailFits80ColsWithLongestTitle pins the header's width contract
// at the narrowest supported terminal: distribution's display title is the
// longest of any configure step (43 cols), so it's the case most likely to
// squeeze the trail — proving the header truncates the title instead.
func TestPhaseTrailFits80ColsWithLongestTitle(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	m := newGoldenModel(t)
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDDistribution})

	frame := tuitest.RenderAt(t, m, 80, 24)
	tuitest.AssertFits(t, frame, 80, 24)

	stripped := tuitest.StripANSI(frame)

	wantTrail := "connect " + tui.IconSuccess + tui.IconActive + tui.IconPending + " · cluster · extras · review"
	if !strings.Contains(stripped, wantTrail) {
		t.Fatalf("frame missing the full, untruncated trail %q:\n%s", wantTrail, stripped)
	}

	longestTitle := "which okd version would you like to deploy?"
	if strings.Contains(stripped, longestTitle) {
		t.Fatalf("frame shows the untruncated title at 80 cols; the header must truncate the title, never the trail:\n%s", stripped)
	}
}
