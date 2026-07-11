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
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

var (
	deployOutputFile          string
	deployMinimal             bool
	deployYes                 bool
	deployDryRun              bool
	deployFresh               bool
	deployMetricsAddr         string
	deployMetricsAllowNetwork bool
)

var deployCmd = &cobra.Command{
	Use:   cmdNameDeploy,
	Short: "Deploy a Kubernetes cluster",
	Long: `Deploy an OKD/OpenShift cluster through an interactive wizard.

Use --yes to write the configuration file non-interactively without
deploying; run the command again without --yes to deploy from it.`,
	Example: `  okdctl deploy
  okdctl deploy --config my-cluster.yaml
  okdctl deploy --yes --output-file my-cluster.yaml  # writes config only; does not deploy
  okdctl deploy --dry-run`,
	RunE: runDeploy,
}

func init() {
	deployCmd.Flags().StringVar(&deployOutputFile, flagOutputFile, "okdctl.yaml", "config file to write wizard output to; reuses an existing file at this path, otherwise creates one; overrides --config when both are set")
	deployCmd.Flags().BoolVar(&deployMinimal, "minimal", false, "use minimal defaults (single-node cluster)")
	deployCmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "write configuration non-interactively; does not deploy")
	deployCmd.Flags().BoolVar(&deployDryRun, flagDryRun, false, "preview terraform plan and step listing without deploying")
	deployCmd.Flags().BoolVar(&deployFresh, "fresh", false, "wipe the work directory even when live cluster state is detected (credentials will be lost)")
	deployCmd.Flags().StringVar(&deployMetricsAddr, "metrics-addr", "", `address for Prometheus metrics endpoint; bare ":9090" binds 127.0.0.1; disabled when empty`)
	deployCmd.Flags().BoolVar(&deployMetricsAllowNetwork, "metrics-allow-network", false, "allow metrics endpoint to bind on a wildcard address (0.0.0.0 or [::])")
}

func runDeploy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if deployMetricsAllowNetwork && deployMetricsAddr == "" {
		return &errtypes.UsageError{
			Msg: "--metrics-allow-network requires --metrics-addr (the flag has no effect on its own)",
		}
	}

	// Deploy self-initializes the workspace: the embedded Terraform sources
	// are materialized before the wizard or any phase code so an empty
	// directory works. Write-once — a source checkout or hand-edited HCL is
	// never overwritten (see deploy.MaterializeTerraform).
	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}
	created, err := deploy.MaterializeTerraform(projectRoot)
	if err != nil {
		return err
	}
	if len(created) > 0 {
		tui.Info("initialized terraform sources",
			tui.LF("dir", filepath.Join(projectRoot, "infrastructure", "terraform")),
			tui.LF("files", len(created)))
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
			if deployYes {
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

	if deployYes {
		return saveConfig(cfg, deployOutputFile, out)
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

	if err := writeCredentialsEnv(cfg, deployOutputFile); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	// Clear secrets before saving so they never appear in YAML.
	// The defer above is a safety net; this is the primary clear.
	clearConfigCredentials(cfg)

	if err := saveConfig(cfg, deployOutputFile, out); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
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

// runDeployDryRun previews a deploy: runs terraform plan and lists every phase
// step. Requires terraform.tfvars from a prior setup run; absent tfvars causes
// plan failure and exits 2.
func runDeployDryRun(ctx context.Context, cfg *config.Config, w io.Writer) error {
	envPath := credentials.EnvFilePath(deployOutputFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return err
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	defer creds.Zeroize()

	projectRoot, err := resolveWorkspaceRoot()
	if err != nil {
		return err
	}

	lock, err := runlock.Acquire(projectRoot, "deploy --dry-run")
	if err != nil {
		return err
	}
	defer lock.Release()

	prov := proxmox.New(
		proxmox.WithProjectRoot(projectRoot),
		proxmox.WithLogger(tui.SimpleLogger()),
		proxmox.WithEnv(creds.Env()),
	)
	defer prov.ZeroizeEnv()
	if connErr := prov.Connect(ctx, cfg); connErr != nil {
		return &errtypes.ConfigError{Msg: "dry-run: provider connect failed", Err: connErr}
	}

	tui.Info("dry-run: running terraform plan (no changes will be made)")

	tfEnv := phase.GetTerraformEnv(cfg)
	if planErr := prov.PlanOnly(ctx, cfg, proxmox.ProvisionOptions{
		ProjectRoot:  projectRoot,
		TerraformEnv: tfEnv,
	}); planErr != nil {
		return &errtypes.ConfigError{Msg: "dry-run: terraform plan failed", Err: planErr}
	}

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

func saveConfig(cfg *config.Config, path string, w io.Writer) error {
	if result := validateConfig(cfg, w); !result.IsValid() {
		tui.Warn("configuration has validation warnings but will still be saved")
	}

	loader := config.NewLoader()
	if err := loader.Save(cfg, path); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	return nil
}

func runFullDeployment(ctx context.Context, cfg *config.Config, w io.Writer) error {
	if deployDryRun {
		return runDeployDryRun(ctx, cfg, w)
	}

	// Hard gate before any phase code: provider fields flow verbatim into
	// terraform.tfvars HCL literals, so a hand-edited config must be
	// rejected here, not warn-and-proceed like saveConfig does. Required
	// and enum scopes are included because validateProvider no-ops on a
	// non-proxmox type string — a bogus type must not bypass the gate.
	gateScope := config.ScopeRequired | config.ScopeEnums | config.ScopeProvider
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
		ShowStartMessage:    true,
		Credentials:         creds,
		MetricsAddr:         deployMetricsAddr,
		AllowNetworkMetrics: deployMetricsAllowNetwork,
		FreshDeploy:         deployFresh,
		ProjectRoot:         projectRoot,
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
