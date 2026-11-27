package cli

import (
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

// hubOutcome is what one hero-hub session leaves behind.
type hubOutcome struct {
	// Result is the wizard's own terminal state.
	Result wizard.Result
	// Verb is the hub verb that ended the session; see sessionVerb.
	Verb steps.HubVerb
}

func runWizardWithMode(cmd *cobra.Command, cfg *config.Config, configExists bool, saved *credentials.ProxmoxCredentials) (hubOutcome, error) {
	wizardCfg := wizard.DefaultConfig()
	wizardCfg.InitialConfig = cfg
	wizardCfg.ConfigExists = configExists

	built, err := buildWizardStepsWithState(wizardCfg, saved)
	if err != nil {
		return hubOutcome{}, err
	}

	var hub *steps.WelcomeStep
	if len(built.Steps) > 0 {
		if ws, ok := built.Steps[0].(*steps.WelcomeStep); ok {
			hub = ws
		}
	}

	result, err := runHubSession(cmd, built.Steps, cfg)
	return hubOutcome{Result: result, Verb: sessionVerb(&result, hub)}, err
}

// sessionVerb returns the hub verb that ended the session. A session the
// review step ended ran the configure flow whatever row the hub still
// highlights.
func sessionVerb(result *wizard.Result, hub *steps.WelcomeStep) steps.HubVerb {
	if hub == nil || result.ExitStep == wizard.StepIDReview {
		return steps.HubVerbGetStarted
	}
	return hub.SelectedVerb()
}

func runHubSession(cmd *cobra.Command, flowSteps []wizard.WizardStep, cfg *config.Config) (wizard.Result, error) {
	restoreLogs := logutil.Redirect(subprocSink())
	defer restoreLogs()
	return wizard.RunFlow(cmd.Context(), flowSteps, cfg, steps.Chrome())
}

// buildWizardStepsWithState builds the configure flow; saved seeds the
// credential fields.
func buildWizardStepsWithState(wizardCfg wizard.Config, saved *credentials.ProxmoxCredentials) (wizard.BuiltSteps, error) {
	builder := wizard.NewStepBuilder()
	steps.RegisterAll(builder, wizardCfg.InitialConfig)
	built, err := wizard.BuildSteps(wizardCfg, builder)
	if err != nil {
		return wizard.BuiltSteps{}, err
	}

	configureWelcomeStep(built, wizardCfg)

	if wizardCfg.InitialConfig != nil {
		initializeStepsFromConfig(built, wizardCfg.InitialConfig, wizardCfg.ConfigExists)
		steps.SeedCredentials(built, saved)
		if state, ok := built.States[wizard.StepTypeResources].(*steps.ResourcesStepState); ok {
			state.Cfg = wizardCfg.InitialConfig
		}
		configureReviewStep(built, wizardCfg.InitialConfig, wizardCfg.ConfigExists)
	}

	return built, nil
}

// configureWelcomeStep marks the hub's config state, passing the loaded config
// through for the dim slot line when one exists.
func configureWelcomeStep(built wizard.BuiltSteps, wizardCfg wizard.Config) {
	for _, step := range built.Steps {
		if ws, ok := step.(*steps.WelcomeStep); ok {
			if wizardCfg.ConfigExists && wizardCfg.InitialConfig != nil {
				ws.SetExistingConfig(wizardCfg.InitialConfig)
			} else {
				ws.SetConfigExists(wizardCfg.ConfigExists)
			}
			break
		}
	}
}

func configureReviewStep(built wizard.BuiltSteps, cfg *config.Config, configExists bool) {
	for _, step := range built.Steps {
		rs, ok := step.(*steps.ReviewStep)
		if !ok {
			continue
		}
		rs.SetConfig(cfg)
		rs.SetConfigPath(initOutputFile)
		if capacity, ok := built.States[wizard.StepTypeReview].(*steps.WizardCapacitySnapshot); ok {
			rs.SetCapacity(capacity)
		}
		if configExists {
			rs.SetSavedConfig(cfg)
		}
		break
	}
}

// initializeStepsFromConfig seeds every data-driven step's fields from cfg;
// configExists distinguishes a real saved file (an empty field value is an
// intentional blank) from a synthetic defaults-only seed like
// config.DefaultConfig() (an empty value is just a gap, so the step's own
// constructed default survives) — see DataDrivenStep.LoadFromConfig.
func initializeStepsFromConfig(built wizard.BuiltSteps, cfg *config.Config, configExists bool) {
	// A synthetic defaults-only seed carries a placeholder version, not a
	// choice the user made; anchoring the selector (and its "current" chip)
	// on it would misrepresent a fresh run as an edit.
	if cfg.Distribution.Version != "" && configExists {
		for _, step := range built.Steps {
			if ds, ok := step.(*steps.DistributionStep); ok {
				ds.SetSelectedVersion(cfg.Distribution.Version)
				break
			}
		}
	}

	for _, step := range built.Steps {
		if ds, ok := step.(*wizard.DataDrivenStep); ok {
			ds.LoadFromConfig(cfg, configExists)
		}
	}
}
