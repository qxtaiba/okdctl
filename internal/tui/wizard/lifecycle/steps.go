package lifecycle

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// flowStepCount is how many screens NewSteps assembles, the step count the
// frame's split gate is evaluated against. The frame counts only visible
// steps, but above the wizard's 20-row floor both gates agree at every
// count this flow can show; TestLifecycleFlowStepCountMatchesNewSteps pins
// the constant.
const flowStepCount = 7

// frameSize records the terminal a step is laid out against plus the body
// box the frame gives it — neither of which View's own arguments report,
// since the frame calls View with a fixed 1000-row budget and a width its
// caps have already flattened.
type frameSize struct {
	termWidth, termHeight int
	bodyHeight            int
}

// SetTerminalSize records the terminal's own dimensions.
func (f *frameSize) SetTerminalSize(width, height int) {
	f.termWidth, f.termHeight = width, height
}

// splitsFrame reports whether this terminal gives the flow a right-hand pane.
func (f *frameSize) splitsFrame() bool {
	return wizard.SplitsFrame(f.termWidth, f.termHeight, flowStepCount)
}

// NewSteps assembles the Cluster Lifecycle flow's ordered steps. Direct
// construction instead of a StepBuilder registry: the registry's
// indirection earns its keep only with multiple assembly sites.
func NewSteps(st *State, hooks Hooks) []wizard.WizardStep {
	return []wizard.WizardStep{
		NewOpStep(st),
		NewTargetStep(st, hooks),
		NewParamsStep(st),
		NewPreviewStep(st, hooks),
		NewConfirmStep(st),
		NewExecStep(st, hooks),
		NewDoneStep(st, hooks),
	}
}

// Stages returns the lifecycle flow's fixed six-stage breadcrumb, pairing confirm and exec under one run stage.
func Stages() []wizard.Stage {
	return []wizard.Stage{
		{Label: "op", Steps: []wizard.StepID{StepIDOp}},
		{Label: "target", Steps: []wizard.StepID{StepIDTarget}},
		{Label: "params", Steps: []wizard.StepID{StepIDParams}},
		{Label: "plan", Steps: []wizard.StepID{StepIDPreview}},
		{Label: "run", Steps: []wizard.StepID{StepIDConfirm, StepIDExec}},
		{Label: "done", Steps: []wizard.StepID{StepIDDone}},
	}
}

// Chrome returns the Cluster Lifecycle flow's header chrome, pairing the day-2 tagline and cluster badge with the fixed six-stage trail.
func Chrome() wizard.FlowChrome {
	return wizard.FlowChrome{
		Tagline: "day-2 node operations",
		Badge:   func(cfg *config.Config) string { return cfg.Cluster.Name },
		Trail:   wizard.StagesTrail(Stages()),
	}
}
