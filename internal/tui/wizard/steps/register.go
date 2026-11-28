package steps

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// RegisterAll registers every configure step factory on b over the session
// config cfg.
func RegisterAll(b *wizard.StepBuilder, cfg *config.Config) {
	capacity := &WizardCapacitySnapshot{cfg: cfg}
	var proxmox *wizard.DataDrivenStep
	secrets := func() wizard.Secrets {
		if proxmox == nil {
			return wizard.Secrets{}
		}
		return proxmox.Secrets()
	}
	b.Register(wizard.StepTypeWelcome, func() (wizard.WizardStep, wizard.StepState) { return NewWelcomeStep(), nil })
	b.Register(wizard.StepTypeDistribution, func() (wizard.WizardStep, wizard.StepState) { return NewDistributionStep(), nil })
	b.Register(wizard.StepTypeBasics, func() (wizard.WizardStep, wizard.StepState) { return NewBasicsStep(), nil })
	b.Register(wizard.StepTypeProxmox, func() (wizard.WizardStep, wizard.StepState) {
		proxmox = NewProxmoxStep()
		return proxmox, nil
	})
	b.Register(wizard.StepTypeNodePlacement, func() (wizard.WizardStep, wizard.StepState) {
		return NewNodePlacementStep().withConfig(cfg).withCapacity(capacity).withSecrets(secrets), nil
	})
	b.Register(wizard.StepTypeNetworking, func() (wizard.WizardStep, wizard.StepState) { return NewNetworkingStep(capacity), nil })
	b.Register(wizard.StepTypeResources, func() (wizard.WizardStep, wizard.StepState) {
		step, state := NewResourcesStep(capacity)
		state.Cfg = cfg
		return step, state
	})
	b.Register(wizard.StepTypeFiles, func() (wizard.WizardStep, wizard.StepState) { return NewFilesStep(), nil })
	b.Register(wizard.StepTypeAdvanced, func() (wizard.WizardStep, wizard.StepState) { return NewAdvancedStep(), nil })
	b.Register(wizard.StepTypeReview, func() (wizard.WizardStep, wizard.StepState) { return NewReviewStep(), capacity })
}

// SeedCredentials seeds the proxmox step's TLS toggle and node placement's
// discovery from the sidecar init loaded; saved must outlive the flow.
func SeedCredentials(built wizard.BuiltSteps, saved *credentials.ProxmoxCredentials) {
	if saved == nil {
		return
	}
	for _, step := range built.Steps {
		switch s := step.(type) {
		case *wizard.DataDrivenStep:
			if s.ID() == wizard.StepIDProxmox && saved.Insecure {
				s.SetValue(fieldInsecure, valYes)
			}
		case *NodePlacementStep:
			s.saved = saved
		}
	}
}

func configurePhases() []wizard.Stage {
	return []wizard.Stage{
		{Label: "connect", Steps: []wizard.StepID{wizard.StepIDWelcome, wizard.StepIDDistribution, wizard.StepIDProxmox}},
		{Label: "cluster", Steps: []wizard.StepID{wizard.StepIDBasics, wizard.StepIDNodePlacement, wizard.StepIDNetworking, wizard.StepIDResources}},
		{Label: "extras", Steps: []wizard.StepID{wizard.StepIDFiles, wizard.StepIDAdvanced}},
		{Label: "review", Steps: []wizard.StepID{wizard.StepIDReview}},
	}
}

// Chrome returns the configure wizard's chrome with the phase trail.
func Chrome() wizard.FlowChrome {
	chrome := wizard.DefaultChrome()
	chrome.Trail = wizard.StagesTrail(configurePhases())
	return chrome
}
