package cli

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/lifecycle"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// wizardDemoEnv enables README-demo recording mode: blank fields, no sudo
// re-exec (see scripts/demo/record.sh). wizardDemoReleasesEnv set to "fail"
// additionally forces the demo distribution step's release fetch into its
// error state, for the error-state screenshot fixture.
const (
	wizardDemoEnv         = "OKDCTL_WIZARD_DEMO"
	wizardDemoReleasesEnv = "OKDCTL_DEMO_RELEASES"
)

// errWizardExited reports a hub flow that finished assembling after the wizard
// it was meant for had already quit.
var errWizardExited = errors.New("open flow: the wizard has already exited")

// hubOutcome is what one hero-hub session leaves behind.
type hubOutcome struct {
	// Result is the wizard's own terminal state.
	Result wizard.Result
	// Verb is the hub verb that ended the session; see sessionVerb.
	Verb steps.HubVerb
	// DayTwoRan reports that a day-2 flow the hub swapped into began executing,
	// so that op — not the configure flow's save pipeline — is what this
	// session did.
	DayTwoRan bool
	// DayTwo is that flow's outcome: exactly the error `okdctl node manage`
	// would have exited with, so a failed or interrupted op is never silently
	// swallowed by the deploy path.
	DayTwo error
}

func runWizardWithMode(cmd *cobra.Command, cfg *config.Config, configExists bool) (hubOutcome, error) {
	wizardCfg := wizard.DefaultConfig()
	wizardCfg.InitialConfig = cfg
	wizardCfg.ConfigExists = configExists

	built, err := buildWizardStepsWithState(wizardCfg)
	if err != nil {
		return hubOutcome{}, err
	}

	var hub *steps.WelcomeStep
	if len(built.Steps) > 0 {
		if ws, ok := built.Steps[0].(*steps.WelcomeStep); ok {
			hub = ws
		}
	}

	var slot lifecycleSlot
	if hub != nil {
		hub.SetFlows(hubFlows(cmd, cfg, &slot))
	}

	result, err := runHubSession(cmd, built.Steps, cfg)

	// take before anything else reads it: a quit can land while the manage
	// verb's session is still being assembled on a command goroutine, and take
	// is what tells that goroutine to close its own session instead.
	manage := slot.take()
	if manage != nil {
		defer manage.close()
	}

	outcome := hubOutcome{Result: result, Verb: sessionVerb(result, hub)}
	// Only a day-2 flow that actually began executing has an outcome to report:
	// one the operator previewed and escaped out of changed nothing, and
	// reporting on it would print a recap over a session that went on to do
	// something else entirely.
	if manage != nil && manage.state.Started {
		outcome.DayTwoRan = true
		outcome.DayTwo = reportLifecycleOutcome(cmd, result, manage.state)
	}

	return outcome, err
}

// sessionVerb returns the hub verb that ended the session. A session the
// review step ended ran the configure flow whatever row the hub still
// highlights.
func sessionVerb(result wizard.Result, hub *steps.WelcomeStep) steps.HubVerb {
	if hub == nil || result.ExitStep == wizard.StepIDReview {
		return steps.HubVerbGetStarted
	}
	return hub.SelectedVerb()
}

func runHubSession(cmd *cobra.Command, flowSteps []wizard.WizardStep, cfg *config.Config) (wizard.Result, error) {
	restoreLogs := logutil.Redirect(subprocSink())
	defer restoreLogs()
	progressBars := logutil.ProgressBarsEnabled()
	logutil.SetProgressBarsEnabled(false)
	defer logutil.SetProgressBarsEnabled(progressBars)
	return wizard.RunFlow(cmd.Context(), flowSteps, cfg, steps.Chrome())
}

// hubFlows builds the hub's in-process flow provider. It is called at the
// moment its verb is confirmed — on a bubbletea command goroutine, not the main
// path — so a `okdctl deploy` that never leaves the configure flow does not pay
// for it. The manage-nodes session is published through slot, which decides
// who closes it: the main path if the wizard is still running, or this
// goroutine if the wizard has already exited underneath it.
func hubFlows(cmd *cobra.Command, cfg *config.Config, slot *lifecycleSlot) steps.HubFlows {
	return steps.HubFlows{
		ManageNodes: func() ([]wizard.WizardStep, wizard.FlowChrome, error) {
			sess, err := newLifecycleSession(cmd, cfg)
			if err != nil {
				return nil, wizard.FlowChrome{}, err
			}
			if !slot.put(sess) {
				// put closed it: the wizard exited while this was assembling,
				// so there is no session left to show a flow for.
				return nil, wizard.FlowChrome{}, errWizardExited
			}
			return sess.steps, lifecycle.Chrome(), nil
		},
	}
}

func buildWizardStepsWithState(wizardCfg wizard.Config) (wizard.BuiltSteps, error) {
	builder := wizard.NewStepBuilder()
	steps.RegisterAll(builder)
	built, err := wizard.BuildSteps(wizardCfg, builder)
	if err != nil {
		return wizard.BuiltSteps{}, err
	}

	configureWelcomeStep(built, wizardCfg)
	configureDemoVersionFetcher(built)

	if wizardCfg.InitialConfig != nil {
		if os.Getenv(wizardDemoEnv) == "" {
			initializeStepsFromConfig(built, wizardCfg.InitialConfig, wizardCfg.ConfigExists)
		}
		configureReviewStep(built, wizardCfg.InitialConfig, wizardCfg.ConfigExists)
	}

	return built, nil
}

// configureWelcomeStep marks the hub's config state, passing the loaded config
// and its save-slot state through for the dim slot line when one exists.
func configureWelcomeStep(built wizard.BuiltSteps, wizardCfg wizard.Config) {
	for _, step := range built.Steps {
		if ws, ok := step.(*steps.WelcomeStep); ok {
			if wizardCfg.ConfigExists && wizardCfg.InitialConfig != nil {
				ws.SetExistingConfig(wizardCfg.InitialConfig, saveSlotState(wizardCfg.InitialConfig))
			} else {
				ws.SetConfigExists(wizardCfg.ConfigExists)
			}
			break
		}
	}
}

// saveSlotState reports how far the saved configuration has got, using only
// what local files can honestly prove: an install marker means a deploy is
// mid-flight, terraform state holding resources means infrastructure exists,
// and anything else is a configuration and nothing more. Probes no network and
// reads no credential — the hub's slot line must never overstate a cluster.
func saveSlotState(cfg *config.Config) steps.SaveSlotState {
	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return steps.SaveSlotConfigured
	}
	if deploy.InstallInProgress(workspace.WorkDir(projectRoot), cfg.Cluster.Name) {
		return steps.SaveSlotDeploying
	}
	if clusterstatus.TerraformStateHasResources(projectRoot, cfg.TerraformEnvName()) {
		return steps.SaveSlotDeployed
	}
	return steps.SaveSlotConfigured
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

func configureReviewStep(built wizard.BuiltSteps, cfg *config.Config, configExists bool) {
	for _, step := range built.Steps {
		rs, ok := step.(*steps.ReviewStep)
		if !ok {
			continue
		}
		rs.SetConfig(cfg)
		rs.SetConfigPath(deployOutputFile)
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

	if state, ok := built.States[wizard.StepTypeResources].(*steps.ResourcesStepState); ok {
		state.Cfg = cfg
	}
}
