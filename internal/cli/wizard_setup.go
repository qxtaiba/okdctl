package cli

import (
	"context"
	"os"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

// wizardDemoEnv puts the wizard in recording mode for the README demo:
// fields start blank (defaults render as placeholders, not values) and
// deploy skips the sudo re-exec. See scripts/demo/record.sh.
const wizardDemoEnv = "OKDCTL_WIZARD_DEMO"

func runWizardWithMode(ctx context.Context, cfg *config.Config, configExists bool) (wizard.Result, steps.WelcomeMode, error) {
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

	result, err := wizard.Run(ctx, built.Steps, cfg)

	var mode steps.WelcomeMode
	if welcomeStep != nil {
		mode = welcomeStep.GetMode()
	}

	return result, mode, err
}

type stepRegistration struct {
	stepType wizard.StepType
	factory  wizard.StepBuilderFactory
}

var defaultStepRegistrations = []stepRegistration{
	{wizard.StepTypeWelcome, func() (wizard.WizardStep, wizard.StepState) { return steps.NewWelcomeStep(), nil }},
	{wizard.StepTypeDistribution, func() (wizard.WizardStep, wizard.StepState) { return steps.NewDistributionStep(), nil }},
	{wizard.StepTypeBasics, func() (wizard.WizardStep, wizard.StepState) { return steps.NewBasicsStep(), nil }},
	{wizard.StepTypeProxmox, func() (wizard.WizardStep, wizard.StepState) { return steps.NewProxmoxStep(), nil }},
	{wizard.StepTypeNodePlacement, func() (wizard.WizardStep, wizard.StepState) { return steps.NewNodePlacementStep(), nil }},
	{wizard.StepTypeNetworking, func() (wizard.WizardStep, wizard.StepState) { return steps.NewNetworkingStep(), nil }},
	{wizard.StepTypeResources, func() (wizard.WizardStep, wizard.StepState) { return steps.NewResourcesStep() }},
	{wizard.StepTypeAddons, func() (wizard.WizardStep, wizard.StepState) { return steps.NewAddonsStep(), nil }},
	{wizard.StepTypeFiles, func() (wizard.WizardStep, wizard.StepState) { return steps.NewFilesStep(), nil }},
	{wizard.StepTypeAdvanced, func() (wizard.WizardStep, wizard.StepState) { return steps.NewAdvancedStep(), nil }},
	{wizard.StepTypeReview, func() (wizard.WizardStep, wizard.StepState) { return steps.NewReviewStep(), nil }},
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
		if os.Getenv(wizardDemoEnv) == "" {
			initializeStepsFromConfig(built, wizardCfg.InitialConfig)
		}
		configureReviewStep(built, wizardCfg.InitialConfig)
	}

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
