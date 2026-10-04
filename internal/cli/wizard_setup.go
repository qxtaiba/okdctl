package cli

import (
	"context"
	"errors"
	"os"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

// wizardDemoEnv enables README-demo recording mode: blank fields, no sudo
// re-exec (see scripts/demo/record.sh).
const wizardDemoEnv = "OKDCTL_WIZARD_DEMO"
const wizardDemoReleasesEnv = "OKDCTL_DEMO_RELEASES"

func runWizardWithMode(ctx context.Context, cfg *config.Config, configExists bool) (wizard.Result, steps.WelcomeMode, error) {
	wizardCfg := wizard.DefaultConfig()
	wizardCfg.InitialConfig = cfg
	wizardCfg.ConfigExists = configExists

	built, welcome, err := buildWizardStepsWithState(wizardCfg)
	if err != nil {
		return wizard.Result{}, steps.WelcomeMode(0), err
	}
	result, err := wizard.Run(ctx, built.Steps, cfg)
	var mode steps.WelcomeMode
	if welcome != nil {
		mode = welcome.GetMode()
	}
	return result, mode, err
}

func buildWizardStepsWithState(wizardCfg wizard.Config) (wizard.BuiltSteps, *steps.WelcomeStep, error) {
	builder := wizard.NewStepBuilder()
	cfg := wizardCfg.InitialConfig
	seed := cfg != nil && os.Getenv(wizardDemoEnv) == ""
	var welcome *steps.WelcomeStep
	builder.Register(wizard.StepTypeWelcome, func() (wizard.WizardStep, wizard.StepState) {
		welcome = steps.NewWelcomeStep()
		welcome.SetConfigExists(wizardCfg.ConfigExists)
		return welcome, nil
	})
	builder.Register(wizard.StepTypeDistribution, func() (wizard.WizardStep, wizard.StepState) {
		step := steps.NewDistributionStep()
		if os.Getenv(wizardDemoEnv) != "" {
			var fetcher steps.VersionFetcher = steps.StaticVersionFetcher{Series: steps.DemoReleaseSeries()}
			if os.Getenv(wizardDemoReleasesEnv) == "fail" {
				fetcher = steps.StaticVersionFetcher{Err: errors.New("demo: releases unavailable")}
			}
			step.SetVersionFetcher(fetcher)
		}
		if seed && cfg.Distribution.Version != "" {
			step.SetSelectedVersion(cfg.Distribution.Version)
		}
		return step, nil
	})
	registerForm := func(kind wizard.StepType, construct func() *wizard.DataDrivenStep) {
		builder.Register(kind, func() (wizard.WizardStep, wizard.StepState) {
			step := construct()
			if seed {
				step.LoadFromConfig(cfg)
			}
			return step, nil
		})
	}
	registerForm(wizard.StepTypeBasics, steps.NewBasicsStep)
	registerForm(wizard.StepTypeProxmox, steps.NewProxmoxStep)
	registerForm(wizard.StepTypeNetworking, steps.NewNetworkingStep)
	registerForm(wizard.StepTypeAddons, steps.NewAddonsStep)
	registerForm(wizard.StepTypeFiles, steps.NewFilesStep)
	registerForm(wizard.StepTypeAdvanced, steps.NewAdvancedStep)
	builder.Register(wizard.StepTypeNodePlacement, func() (wizard.WizardStep, wizard.StepState) { return steps.NewNodePlacementStep(), nil })
	builder.Register(wizard.StepTypeResources, func() (wizard.WizardStep, wizard.StepState) {
		step, state := steps.NewResourcesStep()
		if seed {
			step.LoadFromConfig(cfg)
			state.Cfg = cfg
		}
		return step, state
	})
	builder.Register(wizard.StepTypeReview, func() (wizard.WizardStep, wizard.StepState) {
		step := steps.NewReviewStep()
		if cfg != nil {
			step.SetConfig(cfg)
		}
		return step, nil
	})
	built, err := wizard.BuildSteps(wizardCfg, builder)
	return built, welcome, err
}
