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

// deployPhases groups every configure-wizard step into the header trail's
// four phases: connect, cluster, extras, and review.
func deployPhases() []wizard.Stage {
	return []wizard.Stage{
		{Label: "connect", Steps: []wizard.StepID{wizard.StepIDWelcome, wizard.StepIDDistribution, wizard.StepIDProxmox}},
		{Label: "cluster", Steps: []wizard.StepID{wizard.StepIDBasics, wizard.StepIDNodePlacement, wizard.StepIDNetworking, wizard.StepIDResources}},
		{Label: "extras", Steps: []wizard.StepID{wizard.StepIDAddons, wizard.StepIDFiles, wizard.StepIDAdvanced}},
		{Label: "review", Steps: []wizard.StepID{wizard.StepIDReview}},
	}
}

// Chrome returns the configure wizard's header chrome: wizard.DefaultChrome extended with the named phase trail.
func Chrome() wizard.FlowChrome {
	chrome := wizard.DefaultChrome()
	chrome.Trail = wizard.StagesTrail(deployPhases())
	return chrome
}
