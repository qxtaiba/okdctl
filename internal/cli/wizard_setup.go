package cli

import (
	"context"
	"os"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

// wizardDemoEnv enables README-demo recording mode: blank fields, no sudo
// re-exec (see scripts/demo/record.sh).
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

func buildWizardStepsWithState(wizardCfg wizard.Config) wizard.BuiltSteps {
	builder := wizard.NewStepBuilder()
	builder.Register(wizard.StepTypeWelcome, func() (wizard.WizardStep, wizard.StepState) { return steps.NewWelcomeStep(), nil })
	builder.Register(wizard.StepTypeDistribution, func() (wizard.WizardStep, wizard.StepState) { return steps.NewDistributionStep(), nil })
	builder.Register(wizard.StepTypeBasics, func() (wizard.WizardStep, wizard.StepState) { return steps.NewBasicsStep(), nil })
	builder.Register(wizard.StepTypeProxmox, func() (wizard.WizardStep, wizard.StepState) { return steps.NewProxmoxStep(), nil })
	builder.Register(wizard.StepTypeNodePlacement, func() (wizard.WizardStep, wizard.StepState) { return steps.NewNodePlacementStep(), nil })
	builder.Register(wizard.StepTypeNetworking, func() (wizard.WizardStep, wizard.StepState) { return steps.NewNetworkingStep(), nil })
	builder.Register(wizard.StepTypeResources, func() (wizard.WizardStep, wizard.StepState) { return steps.NewResourcesStep() })
	builder.Register(wizard.StepTypeAddons, func() (wizard.WizardStep, wizard.StepState) { return steps.NewAddonsStep(), nil })
	builder.Register(wizard.StepTypeFiles, func() (wizard.WizardStep, wizard.StepState) { return steps.NewFilesStep(), nil })
	builder.Register(wizard.StepTypeAdvanced, func() (wizard.WizardStep, wizard.StepState) { return steps.NewAdvancedStep(), nil })
	builder.Register(wizard.StepTypeReview, func() (wizard.WizardStep, wizard.StepState) { return steps.NewReviewStep(), nil })
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
