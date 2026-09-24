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

// wizardDemoReleasesEnv set to "fail" forces the demo distribution step's
// release fetch into its error state, for the error-state screenshot fixture.
const wizardDemoReleasesEnv = "OKDCTL_DEMO_RELEASES"

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
	steps.RegisterAll(builder)
	built := wizard.BuildSteps(wizardCfg, builder)

	configureWelcomeStep(built, wizardCfg.ConfigExists)
	configureDemoVersionFetcher(built)

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

// configureDemoVersionFetcher injects a deterministic release-catalog
// fixture under OKDCTL_WIZARD_DEMO, bypassing the network for demo
// recordings and screenshots; OKDCTL_DEMO_RELEASES=fail additionally forces
// the distribution step's error state.
func configureDemoVersionFetcher(built wizard.BuiltSteps) {
	if os.Getenv(wizardDemoEnv) == "" {
		return
	}

	var fetcher steps.VersionFetcher = steps.StaticVersionFetcher{Series: steps.DemoReleaseSeries()}
	if os.Getenv(wizardDemoReleasesEnv) == "fail" {
		fetcher = steps.StaticVersionFetcher{Err: errors.New("demo: releases unavailable")}
	}

	for _, step := range built.Steps {
		if ds, ok := step.(*steps.DistributionStep); ok {
			ds.SetVersionFetcher(fetcher)
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
