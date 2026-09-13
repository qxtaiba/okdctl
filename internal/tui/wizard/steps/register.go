package steps

import "github.com/qxtaiba/okdctl/internal/tui/wizard"

// RegisterAll registers every configure-wizard step factory on b in the
// order wizard.DefaultConfig lists them.
func RegisterAll(b *wizard.StepBuilder) {
	b.Register(wizard.StepTypeWelcome, func() (wizard.WizardStep, wizard.StepState) { return NewWelcomeStep(), nil })
	b.Register(wizard.StepTypeDistribution, func() (wizard.WizardStep, wizard.StepState) { return NewDistributionStep(), nil })
	b.Register(wizard.StepTypeBasics, func() (wizard.WizardStep, wizard.StepState) { return NewBasicsStep(), nil })
	b.Register(wizard.StepTypeProxmox, func() (wizard.WizardStep, wizard.StepState) { return NewProxmoxStep(), nil })
	b.Register(wizard.StepTypeNodePlacement, func() (wizard.WizardStep, wizard.StepState) { return NewNodePlacementStep(), nil })
	b.Register(wizard.StepTypeNetworking, func() (wizard.WizardStep, wizard.StepState) { return NewNetworkingStep(), nil })
	b.Register(wizard.StepTypeResources, func() (wizard.WizardStep, wizard.StepState) { return NewResourcesStep() })
	b.Register(wizard.StepTypeAddons, func() (wizard.WizardStep, wizard.StepState) { return NewAddonsStep(), nil })
	b.Register(wizard.StepTypeFiles, func() (wizard.WizardStep, wizard.StepState) { return NewFilesStep(), nil })
	b.Register(wizard.StepTypeAdvanced, func() (wizard.WizardStep, wizard.StepState) { return NewAdvancedStep(), nil })
	b.Register(wizard.StepTypeReview, func() (wizard.WizardStep, wizard.StepState) { return NewReviewStep(), nil })
}
