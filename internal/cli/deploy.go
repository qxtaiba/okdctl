package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
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
	deployMetricsAddr         string
	deployMetricsAllowNetwork bool
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a Kubernetes cluster",
	Long:  `Deploy an OKD/OpenShift cluster through an interactive wizard.`,
	Example: `  okdctl deploy
  okdctl deploy --yes --config my-cluster.yaml
  okdctl deploy --dry-run`,
	RunE: runDeploy,
}

func init() {
	deployCmd.Flags().StringVar(&deployOutputFile, flagOutputFile, "okdctl.yaml", "output file for configuration")
	deployCmd.Flags().BoolVar(&deployMinimal, "minimal", false, "use minimal defaults (single-node cluster)")
	deployCmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "skip prompts, use defaults")
	deployCmd.Flags().BoolVar(&deployDryRun, flagDryRun, false, "preview terraform plan and step listing without deploying")
	deployCmd.Flags().StringVar(&deployMetricsAddr, "metrics-addr", "", `address for Prometheus metrics endpoint; bare ":9090" binds 127.0.0.1; disabled when empty`)
	deployCmd.Flags().BoolVar(&deployMetricsAllowNetwork, "metrics-allow-network", false, "allow metrics endpoint to bind on a wildcard address (0.0.0.0 or [::])")
}

func runDeploy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if deployMetricsAllowNetwork && deployMetricsAddr == "" {
		return &errtypes.ConfigError{
			Msg: "--metrics-allow-network requires --metrics-addr (the flag has no effect on its own)",
		}
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
				return fmt.Errorf("cannot proceed in non-interactive mode with invalid config: %w", loadErr)
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
		return fmt.Errorf("wizard failed: %w", err)
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
		return fmt.Errorf("load env file %s: %w", envPath, err)
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	defer creds.Zeroize()

	projectRoot, err := resolveProjectRootOrDie()
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

	fmt.Fprintln(w, DryRunSummary("deploy step listing", deployDryRunSteps()))
	tui.Info("dry-run: re-run without --dry-run to execute deploy")
	return nil
}

// deployDryRunSteps returns the ID/Name for every step across setup, install, and
// postinstall phases in execution order.
func deployDryRunSteps() []DryRunStep {
	return []DryRunStep{
		{ID: string(setup.StepInstallPackages), Name: "install system packages"},
		{ID: string(setup.StepInstallTools), Name: "install external tools"},
		{ID: string(setup.StepEnsureWorkDir), Name: "ensure work directory"},
		{ID: string(setup.StepDownloadTools), Name: "download okd tools"},
		{ID: string(setup.StepGenerateConfig), Name: "generate install config"},
		{ID: string(setup.StepGenerateManifests), Name: "generate manifests"},
		{ID: string(setup.StepGenerateKubeVIP), Name: "generate kube-vip manifests"},
		{ID: string(setup.StepInjectManifests), Name: "inject custom manifests"},
		{ID: string(setup.StepCompactCluster), Name: "inject compact cluster manifests"},
		{ID: string(setup.StepGenerateIgnition), Name: "generate ignition"},
		{ID: string(setup.StepInstallApache), Name: "install apache"},
		{ID: string(setup.StepDeployIgnition), Name: "deploy ignition"},
		{ID: string(setup.StepVerifyWebServer), Name: "verify web server"},
		{ID: string(setup.StepBuildISOs), Name: "build isos"},
		{ID: string(setup.StepUploadISOs), Name: "upload isos"},
		{ID: string(setup.StepGenerateTfvars), Name: "generate terraform variables"},
		{ID: string(setup.StepConfigureHAProxy), Name: "configure haproxy"},
		{ID: string(setup.StepConfigureFirewall), Name: "configure firewall"},
		{ID: string(setup.StepConfigureDNS), Name: "configure dns"},
		{ID: string(install.StepDeployInfra), Name: "deploy infrastructure"},
		{ID: string(install.StepWaitBootstrap), Name: "wait for bootstrap"},
		{ID: string(install.StepStartWorkers), Name: "start worker nodes"},
		{ID: string(install.StepSetupKubeconfig), Name: "setup kubeconfig"},
		{ID: string(install.StepValidateAccess), Name: "validate cluster access"},
		{ID: string(install.StepMonitorInstall), Name: "monitor installation"},
		{ID: string(install.StepSetupAccess), Name: "setup cluster access"},
		{ID: string(postinstall.StepVerifyHealth), Name: "verify cluster health"},
		{ID: string(postinstall.StepCleanupBootstrap), Name: "cleanup bootstrap vm"},
		{ID: string(postinstall.StepVerifyKubeVIP), Name: "verify kube-vip"},
		{ID: string(postinstall.StepDeployProductionDNS), Name: "deploy production dns"},
		{ID: string(postinstall.StepInstallAddons), Name: "install addons"},
	}
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

	envPath := credentials.EnvFilePath(deployOutputFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return fmt.Errorf("load env file %s: %w", envPath, err)
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	defer creds.Zeroize()

	if !creds.IsValid() {
		tui.Warn("no proxmox credentials found")
	} else {
		reportCredentialProvenance(creds)
	}

	return executeFullDeployment(ctx, cfg, deploymentOptions{
		ShowStartMessage:    true,
		Credentials:         creds,
		MetricsAddr:         deployMetricsAddr,
		AllowNetworkMetrics: deployMetricsAllowNetwork,
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
