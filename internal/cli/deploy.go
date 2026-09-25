package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

var (
	deployOutputFile             string
	deployMinimal                bool
	deployYes                    bool
	deployConfirmCluster         string
	deployWriteConfig            bool
	deployDryRun                 bool
	deployFresh                  bool
	deployKeepRedHatCatalogs     bool
	deployAcknowledgeInterrupted bool
	deployNoTUI                  bool
)

// Seams for TTY-free tests; production never reassigns them.
var (
	runWizardFn            = runWizardWithMode
	deployExecuteFn        = deploy.Execute
	terraformPlanPreviewFn = runTerraformPlanPreview
)

var deployCmd = &cobra.Command{
	Use:   cmdNameDeploy,
	Short: "Deploy an OKD cluster",
	Long: `Deploy an OKD cluster through an interactive wizard.

Use --yes with --confirm-cluster to skip the wizard and deploy
non-interactively from an existing configuration file (and its okdctl.env
credential sidecar) — no TTY required, so a failed deploy can be resumed
over SSH or from CI. --confirm-cluster must equal the configured cluster
name, the same guard every other scripted lifecycle command carries.
Use --write-config to write the configuration file non-interactively
without deploying.`,
	Example: `  okdctl deploy
  okdctl deploy --config my-cluster.yaml
  okdctl deploy --yes --confirm-cluster=prod         # scripted deploy from okdctl.yaml, no wizard
  okdctl deploy --write-config --output-file my-cluster.yaml  # writes config only; does not deploy
  okdctl deploy --dry-run
  okdctl deploy --keep-redhat-catalogs`,
	Args: cobra.NoArgs,
	RunE: runDeploy,
}

func init() {
	deployCmd.Flags().StringVar(&deployOutputFile, flagOutputFile, "okdctl.yaml", "config file to write wizard output to (reused if present; overrides --config)")
	deployCmd.Flags().BoolVar(&deployMinimal, "minimal", false, "use minimal defaults (single-node cluster)")
	deployCmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "skip the wizard and deploy from the existing configuration file (requires --confirm-cluster)")
	deployCmd.Flags().StringVar(&deployConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal the config cluster name")
	deployCmd.Flags().BoolVar(&deployWriteConfig, "write-config", false, "write configuration non-interactively; does not deploy")
	deployCmd.MarkFlagsMutuallyExclusive("yes", "write-config")
	deployCmd.Flags().BoolVar(&deployDryRun, flagDryRun, false, "preview terraform plan and step listing without deploying")
	deployCmd.Flags().BoolVar(&deployFresh, "fresh", false, "wipe the work directory even when live cluster state is detected (credentials will be lost)")
	deployCmd.Flags().BoolVar(&deployKeepRedHatCatalogs, "keep-redhat-catalogs", false, "keep the Red Hat OperatorHub catalogsources and the InsightsDisabled alert")
	deployCmd.Flags().BoolVar(&deployAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "deploy despite an in-flight node op marker (deploy would otherwise refuse: reconciling mid-op destroys the in-flight node)")
	deployCmd.Flags().BoolVar(&deployNoTUI, flagNoTUI, false, "stream the install as a plain stderr checklist instead of the full-screen wizard")
}

func runDeploy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	// Materializes the embedded Terraform sources write-once; a source checkout
	// or hand-edited HCL is never overwritten (see
	// deploy.MaterializeTerraform).
	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}
	if err := withProjectLock(projectRoot, "deploy", func() error {
		created, err := deploy.MaterializeTerraform(projectRoot)
		if err != nil {
			return err
		}
		if len(created) > 0 {
			logutil.Info("initialized terraform sources",
				logutil.LF("dir", filepath.Join(projectRoot, "infrastructure", "terraform")),
				logutil.LF("count", len(created)))
		}
		return nil
	}); err != nil {
		return err
	}

	// --output-file wins when explicit; otherwise --config; both default to "okdctl.yaml".
	if !cmd.Flags().Changed(flagOutputFile) && cmd.Root().PersistentFlags().Changed(flagConfig) {
		deployOutputFile = cfgFile
	}

	cfg, configExists, configFileMissing, err := loadDeployConfig(deployOutputFile)
	if err != nil {
		return err
	}

	if deployDryRun {
		// A missing config (as opposed to an invalid one, handled above) must
		// fail fast rather than silently plan compiled-in defaults against
		// whatever terraform workspace happens to sit in cwd — that's a real
		// terraform init touching network and disk, not a preview of anything
		// the operator asked for.
		if !configExists && configFileMissing {
			return errConfigNotFound(deployOutputFile)
		}
		return runDeployDryRun(ctx, cfg, out)
	}

	if deployWriteConfig {
		return withProjectLock(projectRoot, "deploy --write-config", func() error {
			return saveConfig(cfg, deployOutputFile, out)
		})
	}

	// --yes requires an existing config (deploying compiled-in defaults
	// unattended is a footgun) plus --confirm-cluster.
	if deployYes {
		if !configExists {
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf("--yes deploys non-interactively and requires an existing configuration file at %s; run 'okdctl deploy' for the wizard or 'okdctl deploy --write-config' first", deployOutputFile),
				Err: errtypes.ErrConfigMissing,
			}
		}
		if err := confirmClusterMatches(true, deployConfirmCluster, cfg.Cluster.Name, "deploy"); err != nil {
			return err
		}
		return runFullDeployment(ctx, cfg, out)
	}

	outcome, err := runWizardFn(cmd, cfg, configExists)
	if err != nil {
		return (&errtypes.ConfigError{Msg: "wizard failed", Err: err}).
			WithHint("try again, or use --yes with a saved config for a non-interactive deploy")
	}

	// A day-2 flow the hub swapped into and executed owns the session's exit:
	// it has already reported for itself, and nothing it did belongs in the
	// configure flow's save pipeline.
	if outcome.DayTwoRan {
		return outcome.DayTwo
	}

	if outcome.Result.Cancelled {
		logutil.Info("wizard cancelled, no changes made")
		return nil
	}

	if handled, verbErr := runHubVerb(ctx, outcome.Verb, cfg, out); handled {
		return verbErr
	}

	cfg = outcome.Result.Config

	defer clearConfigCredentials(cfg)

	if err := withProjectLock(projectRoot, "deploy", func() error {
		return persistWizardConfig(cfg, deployOutputFile, out)
	}); err != nil {
		return err
	}

	switch outcome.Result.Action {
	case wizard.ActionDeploy:
		if err := runFullDeployment(ctx, cfg, out); err != nil {
			return err
		}
	case wizard.ActionExit:
		fmt.Fprintln(out)
		logutil.Info("configuration saved", logutil.LF("path", deployOutputFile))
	}

	return nil
}

// destroyHandoff is the line the hub's destroy verb prints once the TUI has
// released the terminal; okdctl destroy owns the confirm ladder, which the
// wizard must never re-implement behind a menu entry.
const destroyHandoff = "run: okdctl destroy"

// runHubVerb handles the hub verbs that never walk the configure flow,
// reporting whether verb was one of them so the caller can skip the save
// pipeline entirely. deploy runs the configuration already on disk untouched;
// destroy prints its handoff and lets okdctl destroy's own confirm ladder be
// the guard; the day-2 verbs ran in-process and have already reported.
func runHubVerb(ctx context.Context, verb steps.HubVerb, cfg *config.Config, out io.Writer) (handled bool, err error) {
	switch verb {
	case steps.HubVerbDeploy:
		return true, runFullDeployment(ctx, cfg, out)
	case steps.HubVerbDestroy:
		fmt.Fprintln(out, destroyHandoff)
		return true, nil
	case steps.HubVerbQuit:
		logutil.Info("no changes made")
		return true, nil
	case steps.HubVerbManageNodes, steps.HubVerbClusterStatus:
		return true, nil
	default:
		return false, nil
	}
}

// loadDeployConfig resolves the config deploy runs against — an existing,
// valid on-disk file, or compiled-in defaults when none exists — and
// distinguishes "no file" (configFileMissing) from "file exists but is
// invalid" for the callers (--yes, --write-config, --dry-run) that must
// fail fast on the latter rather than silently falling back to defaults,
// the way an interactive wizard run is allowed to.
func loadDeployConfig(path string) (cfg *config.Config, configExists, configFileMissing bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		configExists = true
		loadedCfg, loadErr := config.NewLoader().LoadFile(path)
		if loadErr != nil {
			logutil.Warn("existing config could not be loaded", logutil.LF("err", loadErr))
			switch {
			case deployYes || deployWriteConfig:
				return nil, false, false, (&errtypes.ConfigError{Msg: "cannot proceed in non-interactive mode with invalid config", Err: loadErr}).
					WithHint("run 'okdctl config validate' to see what's wrong, or drop --yes to use the wizard")
			case deployDryRun:
				// Same hazard as the missing-config short-circuit in
				// runDeploy: the wizard's warn-and-fall-back-to-defaults
				// leniency would silently plan compiled-in defaults against
				// a typo'd config, not preview what's actually on disk.
				return nil, false, false, (&errtypes.ConfigError{Msg: "cannot proceed in non-interactive mode with invalid config", Err: loadErr}).
					WithHint("run 'okdctl config validate' to see what's wrong, or drop --dry-run to use the wizard")
			}
			logutil.Info("starting fresh with defaults")
			configExists = false
		} else {
			cfg = loadedCfg
		}
	} else {
		configFileMissing = true
	}

	if cfg == nil {
		if deployMinimal {
			cfg = config.MinimalConfig()
		} else {
			cfg = config.DefaultConfig()
		}
	}
	return cfg, configExists, configFileMissing, nil
}

// runDeployDryRun previews a deploy via terraform plan and phase step listing;
// it exits 0 even when the plan reports drift ('okdctl plan' is the
// drift-gating surface).
func runDeployDryRun(ctx context.Context, cfg *config.Config, w io.Writer) error {
	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}

	logutil.Info("dry-run: running terraform plan (no changes will be made)")

	changes, err := terraformPlanPreviewFn(ctx, cfg, planPreviewOptions{
		ConfigPath:  deployOutputFile,
		ProjectRoot: projectRoot,
		Caller:      "deploy --dry-run",
	})
	if err != nil {
		// A specific ConfigError (e.g. runlock's "another okdctl process holds
		// the project lock") must pass through as-is — re-wrapping it here would
		// bury its message behind this generic one, since errors.As/Describe
		// only ever look at the outermost ConfigError in the chain.
		var cfgErr *errtypes.ConfigError
		if errors.As(err, &cfgErr) {
			return err
		}
		return &errtypes.ConfigError{Msg: "dry-run: plan preview failed", Err: err}
	}

	printDeployDryRunBoxes(w, changes, cfg, projectRoot)
	logutil.Info("dry-run: re-run without --dry-run to execute deploy")
	return nil
}

// printDeployDryRunBoxes prints the plan-preview box immediately followed by
// the step-listing box: PlanPreview uses Fprint, not Fprintln, because the
// step-listing box's own leading newline already supplies the single blank
// line the two adjacent boxes need between them.
func printDeployDryRunBoxes(w io.Writer, changes []terraform.ResourceChange, cfg *config.Config, projectRoot string) {
	fmt.Fprint(w, render.PlanPreview(changes))
	fmt.Fprintln(w, render.DryRunSummary("deploy step listing", deployDryRunSteps(cfg, projectRoot)))
}

// deployDryRunSteps derives the step listing from live phase StepDefs so it
// cannot drift from what deploy actually runs.
func deployDryRunSteps(cfg *config.Config, projectRoot string) []render.DryRunStep {
	deploySteps := okd.New(okd.WithProjectRoot(projectRoot), okd.WithLogger(logutil.SimpleLogger())).DeploySteps(cfg)
	out := make([]render.DryRunStep, len(deploySteps))
	for i, s := range deploySteps {
		out[i] = render.DryRunStep{ID: string(s.ID), Name: s.Name}
	}
	return out
}

// withProjectLock serializes shared-file writes against concurrent okdctl
// invocations; deploy.Execute re-acquires the lock separately for the
// deployment itself.
func withProjectLock(projectRoot, verb string, fn func() error) error {
	lock, err := runlock.Acquire(projectRoot, verb)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}

// persistWizardConfig writes credentials to the .env sidecar, clears them, then
// saves YAML — in that order, so credential bytes never reach okdctl.yaml.
func persistWizardConfig(cfg *config.Config, path string, w io.Writer) error {
	if err := writeCredentialsEnv(cfg, path); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	clearConfigCredentials(cfg)
	return saveConfig(cfg, path, w)
}

func saveConfig(cfg *config.Config, path string, w io.Writer) error {
	if result := validateConfig(cfg, w); !result.IsValid() {
		logutil.Warn("configuration has validation warnings but will still be saved")
	}

	loader := config.NewLoader()
	if err := loader.Save(cfg, path); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}

	return nil
}

// deployGateScope covers every render surface plus required/enums, so a bogus
// provider type can't bypass validateProvider's no-op.
const deployGateScope = config.ScopeRequired | config.ScopeEnums | config.ScopeProvider |
	config.ScopeAdvancedNetworking | config.ScopeNetworking | config.ScopeHTTPServer

func runFullDeployment(ctx context.Context, cfg *config.Config, w io.Writer) error {
	if deployDryRun {
		return runDeployDryRun(ctx, cfg, w)
	}

	// Resolved before the gate so validation's terraform-env check sees the
	// same path materialization uses.
	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}

	if err := refuseInFlightNodeOp(projectRoot, cfg, deployAcknowledgeInterrupted); err != nil {
		return err
	}

	// Hard gate: a hand-edited config is rejected here, not warn-and-proceed like saveConfig.
	gate := config.ValidationOptions{Scope: deployGateScope, ProjectRoot: projectRoot}
	if result := config.ValidateWithOptions(cfg, gate); !result.IsValid() {
		return (&errtypes.ConfigError{Msg: "config validation failed", Err: result}).
			WithHint("run 'okdctl config validate' to see every failing field")
	}

	envPath := credentials.EnvFilePath(deployOutputFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return err
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	defer creds.Zeroize()

	if !creds.IsValid() {
		logutil.Warn("no proxmox credentials found")
	} else {
		reportCredentialProvenance(creds)
	}

	opts := deploy.Options{
		ShowStartMessage:   true,
		Credentials:        creds,
		FreshDeploy:        deployFresh,
		KeepRedHatCatalogs: deployKeepRedHatCatalogs,
		ProjectRoot:        projectRoot,
		LogSink:            runLogSink,
		Verbose:            logVerbose,
	}
	if deployStreamEnabled() {
		return runDeployStream(ctx, cfg, &opts, w)
	}
	_, execErr := deployExecuteFn(ctx, cfg, &opts, w)
	return execErr
}

func writeCredentialsEnv(cfg *config.Config, configPath string) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}
	px := cfg.Provider.Proxmox

	if px.Password.IsEmpty() && px.APIToken.IsEmpty() {
		return nil
	}

	// Resolve the normalized endpoint so the .env file is self-contained.
	resolved := credentials.GetProxmoxCredentials(cfg)

	creds := &credentials.ProxmoxCredentials{
		Endpoint: resolved.Endpoint,
		Username: px.Username,
		Password: append([]byte(nil), px.Password.Bytes()...),
		APIToken: append([]byte(nil), px.APIToken.Bytes()...),
		Insecure: px.Insecure,
	}
	defer creds.Zeroize()

	envPath := credentials.EnvFilePath(configPath)
	if err := credentials.WriteEnvFile(envPath, creds); err != nil {
		return err
	}

	logutil.Info("credentials saved", logutil.LF("path", envPath))
	return nil
}

// clearConfigCredentials zeroizes credential bytes so they are never serialized
// or left lingering on the heap.
func clearConfigCredentials(cfg *config.Config) {
	if cfg.Provider.Proxmox == nil {
		return
	}
	cfg.Provider.Proxmox.Password.Zeroize()
	cfg.Provider.Proxmox.APIToken.Zeroize()
}
