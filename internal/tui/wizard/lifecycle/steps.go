package lifecycle

import "github.com/qxtaiba/okdctl/internal/tui/wizard"

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
		NewDoneStep(st),
	}
}
