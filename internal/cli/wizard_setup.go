package cli

import (
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/steps"
)

func runWizardWithMode(cfg *config.Config, configExists bool) (wizard.Result, steps.WelcomeMode, error) {
	wizardCfg := wizard.DefaultConfig()
	wizardCfg.InitialConfig = cfg
	wizardCfg.ConfigExists = configExists

	built := buildWizardStepsWithState(wizardCfg)

	var welcomeStep *steps.WelcomeStep
	if len(built.Steps) > 0 {
		if ws, ok := built.Steps[0].(*steps.WelcomeStep); ok {
			welcomeStep = ws
		}
	}

	result, err := wizard.Run(built.Steps, cfg)

	var mode steps.WelcomeMode
	if welcomeStep != nil {
		mode = welcomeStep.GetMode()
	}

	return result, mode, err
}

type stepRegistration struct {
	stepType wizard.StepType
	factory  func() (wizard.WizardStep, any)
}

var defaultStepRegistrations = []stepRegistration{
	{wizard.StepTypeWelcome, func() (wizard.WizardStep, any) { return steps.NewWelcomeStep(), nil }},
	{wizard.StepTypeDistribution, func() (wizard.WizardStep, any) { return steps.NewDistributionStep(), nil }},
	{wizard.StepTypeBasics, func() (wizard.WizardStep, any) { return steps.NewBasicsStep() }},
	{wizard.StepTypeProxmox, func() (wizard.WizardStep, any) { return steps.NewProxmoxStep() }},
	{wizard.StepTypeNetworking, func() (wizard.WizardStep, any) { return steps.NewNetworkingStep() }},
	{wizard.StepTypeResources, func() (wizard.WizardStep, any) { return steps.NewResourcesStep() }},
	{wizard.StepTypeAddons, func() (wizard.WizardStep, any) { return steps.NewAddonsStep() }},
	{wizard.StepTypeFiles, func() (wizard.WizardStep, any) { return steps.NewFilesStep() }},
	{wizard.StepTypeAdvanced, func() (wizard.WizardStep, any) { return steps.NewAdvancedStep() }},
	{wizard.StepTypeReview, func() (wizard.WizardStep, any) { return steps.NewReviewStep(), nil }},
}

func newDefaultStepBuilder() *wizard.StepBuilder {
	builder := wizard.NewStepBuilder()
	for _, reg := range defaultStepRegistrations {
		builder.Register(reg.stepType, reg.factory)
	}
	return builder
}

func buildWizardStepsWithState(wizardCfg wizard.Config) wizard.BuiltSteps {
	builder := newDefaultStepBuilder()
	built := wizard.BuildSteps(wizardCfg, builder)

	configureWelcomeStep(built, wizardCfg.ConfigExists)

	if wizardCfg.InitialConfig != nil {
		initializeStepsFromConfig(built, wizardCfg.InitialConfig)
	}

	configureReviewStep(built, wizardCfg.InitialConfig)

	return built
}

func configureWelcomeStep(built wizard.BuiltSteps, configExists bool) {
	for _, step := range built.Steps {
		if ws, ok := step.(*steps.WelcomeStep); ok {
			ws.SetConfigExists(configExists)
			break
		}
	}
}

func configureReviewStep(built wizard.BuiltSteps, cfg *config.Config) {
	for _, step := range built.Steps {
		if rs, ok := step.(*steps.ReviewStep); ok {
			rs.SetConfig(cfg)
			break
		}
	}
}

func initializeStepsFromConfig(built wizard.BuiltSteps, cfg *config.Config) {
	if cfg.Distribution.Version != "" {
		for _, step := range built.Steps {
			if ds, ok := step.(*steps.DistributionStep); ok {
				ds.SetSelectedVersion(cfg.Distribution.Version)
				break
			}
		}
	}

	for _, step := range built.Steps {
		if ds, ok := step.(*wizard.DataDrivenStep); ok {
			ds.LoadFromConfig(cfg)
		}
	}

	if state, ok := built.States[wizard.StepTypeResources].(*steps.ResourcesStepState); ok {
		state.Cfg = cfg
	}
}
