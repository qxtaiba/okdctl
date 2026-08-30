package cli

import (
	"context"
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
)

// Seams for TTY-free tests; production never reassigns them.
var (
	runWizardFn     = runWizardWithMode
	deployExecuteFn = deploy.Execute
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
	deployCmd.Flags().StringVar(&deployOutputFile, flagOutputFile, "okdctl.yaml", "config file to write wizard output to; reuses and reads back an existing file at this path, otherwise creates one; overrides --config when both are set")
	deployCmd.Flags().BoolVar(&deployMinimal, "minimal", false, "use minimal defaults (single-node cluster)")
	deployCmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "skip the wizard and deploy from the existing configuration file (requires --confirm-cluster)")
	deployCmd.Flags().StringVar(&deployConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal the config cluster name")
	deployCmd.Flags().BoolVar(&deployWriteConfig, "write-config", false, "write configuration non-interactively; does not deploy")
	deployCmd.MarkFlagsMutuallyExclusive("yes", "write-config")
	deployCmd.Flags().BoolVar(&deployDryRun, flagDryRun, false, "preview terraform plan and step listing without deploying")
	deployCmd.Flags().BoolVar(&deployFresh, "fresh", false, "wipe the work directory even when live cluster state is detected (credentials will be lost)")
	deployCmd.Flags().BoolVar(&deployKeepRedHatCatalogs, "keep-redhat-catalogs", false, "keep the redhat-operators, certified-operators, and redhat-marketplace OperatorHub catalogsources and the InsightsDisabled alert enabled (both require a Red Hat subscription OKD clusters don't have)")
	deployCmd.Flags().BoolVar(&deployAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "deploy despite an in-flight node op marker (deploy would otherwise refuse: reconciling mid-op destroys the in-flight node)")
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

	configExists := false
	var cfg *config.Config

	if _, err := os.Stat(deployOutputFile); err == nil {
		configExists = true
		loader := config.NewLoader()
		loadedCfg, loadErr := loader.LoadFile(deployOutputFile)
		if loadErr != nil {
			logutil.Warn("existing config could not be loaded", logutil.LF("err", loadErr))
			if deployYes || deployWriteConfig {
				return &errtypes.ConfigError{Msg: "cannot proceed in non-interactive mode with invalid config", Err: loadErr}
			}
			logutil.Info("starting fresh with defaults")
			configExists = false
		} else {
			cfg = loadedCfg
		}
	}

	if cfg == nil {
		if deployMinimal {
			cfg = config.MinimalConfig()
		} else {
			cfg = config.DefaultConfig()
		}
	}

	if deployDryRun {
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

	result, welcomeMode, err := runWizardFn(ctx, cfg, configExists)
	if err != nil {
		return &errtypes.ConfigError{Msg: "wizard failed", Err: err}
	}

	if result.Cancelled {
		logutil.Info("wizard cancelled, no changes made")
		return nil
	}

	if welcomeMode == steps.WelcomeModeDeploy {
		return runFullDeployment(ctx, cfg, out)
	}

	cfg = result.Config

	defer clearConfigCredentials(cfg)

	if err := withProjectLock(projectRoot, "deploy", func() error {
		return persistWizardConfig(cfg, deployOutputFile, out)
	}); err != nil {
		return err
	}

	switch result.Action {
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

// runDeployDryRun previews a deploy via terraform plan and phase step listing;
// it exits 0 even when the plan reports drift ('okdctl plan' is the
// drift-gating surface).
func runDeployDryRun(ctx context.Context, cfg *config.Config, w io.Writer) error {
	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}

	logutil.Info("dry-run: running terraform plan (no changes will be made)")

	changes, err := runTerraformPlanPreview(ctx, cfg, planPreviewOptions{
		ConfigPath:  deployOutputFile,
		ProjectRoot: projectRoot,
		Caller:      "deploy --dry-run",
	})
	if err != nil {
		return &errtypes.ConfigError{Msg: "dry-run: plan preview failed", Err: err}
	}

	fmt.Fprint(w, render.PlanPreview(changes))
	fmt.Fprintln(w, render.DryRunSummary("deploy step listing", deployDryRunSteps(cfg, projectRoot)))
	logutil.Info("dry-run: re-run without --dry-run to execute deploy")
	return nil
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
		return &errtypes.ConfigError{Msg: "config validation failed", Err: result}
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

	return deployExecuteFn(ctx, cfg, deploy.Options{
		ShowStartMessage:   true,
		Credentials:        creds,
		FreshDeploy:        deployFresh,
		KeepRedHatCatalogs: deployKeepRedHatCatalogs,
		ProjectRoot:        projectRoot,
		LogSink:            runLogSink,
		Verbose:            logVerbose,
	}, w)
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
