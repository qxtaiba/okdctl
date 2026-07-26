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
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

var (
	deployOutputFile         string
	deployMinimal            bool
	deployYes                bool
	deployWriteConfig        bool
	deployDryRun             bool
	deployFresh              bool
	deployKeepRedHatCatalogs bool
)

var deployCmd = &cobra.Command{
	Use:   cmdNameDeploy,
	Short: "Deploy a Kubernetes cluster",
	Long: `Deploy an OKD/OpenShift cluster through an interactive wizard.

Use --yes to skip the wizard and deploy non-interactively from an existing
configuration file. Use --write-config to write the configuration file
non-interactively without deploying.

Note: before v0.2.0, --yes meant what --write-config means now.`,
	Example: `  okdctl deploy
  okdctl deploy --config my-cluster.yaml
  okdctl deploy --yes                                # deploys from okdctl.yaml, no wizard
  okdctl deploy --write-config --output-file my-cluster.yaml  # writes config only; does not deploy
  okdctl deploy --dry-run
  okdctl deploy --keep-redhat-catalogs`,
	RunE: runDeploy,
}

func init() {
	deployCmd.Flags().StringVar(&deployOutputFile, flagOutputFile, "okdctl.yaml", "config file to write wizard output to; reuses and reads back an existing file at this path, otherwise creates one; overrides --config when both are set")
	deployCmd.Flags().BoolVar(&deployMinimal, "minimal", false, "use minimal defaults (single-node cluster)")
	deployCmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "skip the wizard and deploy from the existing configuration file")
	deployCmd.Flags().BoolVar(&deployWriteConfig, "write-config", false, "write configuration non-interactively; does not deploy")
	deployCmd.MarkFlagsMutuallyExclusive("yes", "write-config")
	deployCmd.Flags().BoolVar(&deployDryRun, flagDryRun, false, "preview terraform plan and step listing without deploying")
	deployCmd.Flags().BoolVar(&deployFresh, "fresh", false, "wipe the work directory even when live cluster state is detected (credentials will be lost)")
	deployCmd.Flags().BoolVar(&deployKeepRedHatCatalogs, "keep-redhat-catalogs", false, "keep the redhat-operators, certified-operators, and redhat-marketplace OperatorHub catalogsources and the InsightsDisabled alert enabled (both require a Red Hat subscription OKD clusters don't have)")
}

func runDeploy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	// Deploy self-initializes the workspace: the embedded Terraform sources
	// are materialized before the wizard or any phase code so an empty
	// directory works. Write-once — a source checkout or hand-edited HCL is
	// never overwritten (see deploy.MaterializeTerraform).
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
			tui.Info("initialized terraform sources",
				tui.LF("dir", filepath.Join(projectRoot, "infrastructure", "terraform")),
				tui.LF("files", len(created)))
		}
		return nil
	}); err != nil {
		return err
	}

	// Resolve the config file path: --output-file wins when explicitly set;
	// otherwise honour --config when the caller provided it; fall back to the
	// --output-file default ("okdctl.yaml") which matches --config's default.
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
			tui.Warn("existing config could not be loaded", tui.LF("err", loadErr))
			if deployYes || deployWriteConfig {
				return &errtypes.ConfigError{Msg: "cannot proceed in non-interactive mode with invalid config", Err: loadErr}
			}
			tui.Info("starting fresh with defaults")
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

	// --yes carries the same assume-yes meaning as every sibling command:
	// perform the operation without interaction. It requires a config that
	// already exists on disk — deploying compiled-in defaults unattended
	// would be a footgun, and 66 (config missing) tells scripts exactly why.
	if deployYes {
		if !configExists {
			return &errtypes.ConfigError{
				Msg: fmt.Sprintf("--yes deploys non-interactively and requires an existing configuration file at %s; run 'okdctl deploy' for the wizard or 'okdctl deploy --write-config' first", deployOutputFile),
				Err: errtypes.ErrConfigMissing,
			}
		}
		return runFullDeployment(ctx, cfg, out)
	}

	result, welcomeMode, err := runWizardWithMode(ctx, cfg, configExists)
	if err != nil {
		return &errtypes.ConfigError{Msg: "wizard failed", Err: err}
	}

	if result.Cancelled {
		tui.Info("wizard cancelled, no changes made")
		return nil
	}

	if welcomeMode == steps.WelcomeModeDeploy {
		return runFullDeployment(ctx, cfg, out)
	}

	cfg = result.Config

	// Guarantee secrets are cleared from the config struct, even on panic.
	defer clearConfigCredentials(cfg)

	if err := withProjectLock(projectRoot, "deploy", func() error {
		if err := writeCredentialsEnv(cfg, deployOutputFile); err != nil {
			return fmt.Errorf("save credentials: %w", err)
		}

		// Clear secrets before saving so they never appear in YAML.
		// The defer above is a safety net; this is the primary clear.
		clearConfigCredentials(cfg)

		return saveConfig(cfg, deployOutputFile, out)
	}); err != nil {
		return err
	}

	switch result.Action {
	case wizard.ActionDeploy:
		if err := runFullDeployment(ctx, cfg, out); err != nil {
			return err
		}
	case wizard.ActionExit:
		showExitSummary(deployOutputFile, out)
	}

	return nil
}

// runDeployDryRun previews a deploy: runs a terraform plan preview and lists
// every phase step. Requires terraform.tfvars from a prior setup run; absent
// tfvars causes plan failure and exits 2. Always exits 0 when the preview
// itself succeeds, even when the plan reports drift — unlike 'okdctl plan',
// dry-run's exit contract predates drift-awareness and stays unchanged here.
func runDeployDryRun(ctx context.Context, cfg *config.Config, w io.Writer) error {
	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}

	tui.Info("dry-run: running terraform plan (no changes will be made)")

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
	tui.Info("dry-run: re-run without --dry-run to execute deploy")
	return nil
}

// deployDryRunSteps returns the ID/Name for every step across setup, install,
// and postinstall phases in execution order, derived from the live phase
// StepDefs via okd.Provisioner.DeploySteps so this listing cannot drift from
// what deploy actually runs.
func deployDryRunSteps(cfg *config.Config, projectRoot string) []render.DryRunStep {
	deploySteps := okd.New(okd.WithProjectRoot(projectRoot), okd.WithLogger(tui.SimpleLogger())).DeploySteps(cfg)
	out := make([]render.DryRunStep, len(deploySteps))
	for i, s := range deploySteps {
		out[i] = render.DryRunStep{ID: string(s.ID), Name: s.Name}
	}
	return out
}

// withProjectLock runs fn while holding the project runlock. runDeploy uses
// it to serialize its shared-file write windows (terraform materialization,
// okdctl.yaml, okdctl.env) against concurrent okdctl invocations while
// keeping the interactive wizard outside the lock; deploy.Execute takes the
// lock again for the deployment itself.
func withProjectLock(projectRoot, verb string, fn func() error) error {
	lock, err := runlock.Acquire(projectRoot, verb)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}

func saveConfig(cfg *config.Config, path string, w io.Writer) error {
	if result := validateConfig(cfg, w); !result.IsValid() {
		tui.Warn("configuration has validation warnings but will still be saved")
	}

	loader := config.NewLoader()
	if err := loader.Save(cfg, path); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}

	return nil
}

func runFullDeployment(ctx context.Context, cfg *config.Config, w io.Writer) error {
	if deployDryRun {
		return runDeployDryRun(ctx, cfg, w)
	}

	// Hard gate before any phase code: provider fields flow verbatim into
	// terraform.tfvars HCL literals, and the bastion identity (Bastion.IP,
	// StaticIP.DNS, HTTPServer.IgnitionServerIP) flows into apache's bind
	// address and every node's static-ip kernel args, so a hand-edited
	// config must be rejected here, not warn-and-proceed like saveConfig
	// does. Required and enum scopes are included because validateProvider
	// no-ops on a non-proxmox type string — a bogus type must not bypass
	// the gate.
	gateScope := config.ScopeRequired | config.ScopeEnums | config.ScopeProvider | config.ScopeAdvancedNetworking
	if result := config.ValidateWithOptions(cfg, config.ValidationOptions{Scope: gateScope}); !result.IsValid() {
		return &errtypes.ConfigError{Msg: "config validation failed", Err: result}
	}

	envPath := credentials.EnvFilePath(deployOutputFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return err
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	defer creds.Zeroize()

	if !creds.IsValid() {
		tui.Warn("no proxmox credentials found")
	} else {
		reportCredentialProvenance(creds)
	}

	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}

	return deploy.Execute(ctx, cfg, deploy.Options{
		ShowStartMessage:   true,
		Credentials:        creds,
		FreshDeploy:        deployFresh,
		KeepRedHatCatalogs: deployKeepRedHatCatalogs,
		ProjectRoot:        projectRoot,
		LogSink:            runLogSink,
		Verbose:            logVerbose,
	}, w)
}

func showExitSummary(path string, w io.Writer) {
	fmt.Fprintln(w)
	tui.Info("configuration saved", tui.LF("path", path))
}

func writeCredentialsEnv(cfg *config.Config, configPath string) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}
	px := cfg.Provider.Proxmox

	if px.Password.IsEmpty() && px.APIToken.IsEmpty() {
		return nil
	}

	// Resolve the normalized endpoint (adds https:// and :8006 as needed)
	// so the .env file is self-contained for Proxmox connection.
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

	tui.Info("credentials saved", tui.LF("path", envPath))
	return nil
}

// clearConfigCredentials wipes the in-memory credential bytes so they are
// never serialized to YAML and do not linger as Go strings on the heap.
func clearConfigCredentials(cfg *config.Config) {
	if cfg.Provider.Proxmox == nil {
		return
	}
	cfg.Provider.Proxmox.Password.Zeroize()
	cfg.Provider.Proxmox.APIToken.Zeroize()
}
