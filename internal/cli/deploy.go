package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

var (
	deployOutputFile  string
	deployMinimal     bool
	deployYes         bool
	deployDryRun      bool
	deployMetricsAddr string
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a Kubernetes cluster",
	Long:  `Deploy an OKD/OpenShift cluster through an interactive wizard.`,
	RunE:  runDeploy,
}

func init() {
	deployCmd.Flags().StringVarP(&deployOutputFile, "output", "o", "okdctl.yaml", "output file for configuration")
	deployCmd.Flags().BoolVar(&deployMinimal, "minimal", false, "use minimal defaults (single-node cluster)")
	deployCmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "skip prompts, use defaults")
	deployCmd.Flags().BoolVar(&deployDryRun, "dry-run", false, "preview terraform plan and step listing without deploying")
	deployCmd.Flags().StringVar(&deployMetricsAddr, "metrics-addr", "", "address for Prometheus metrics endpoint (e.g. :9090); disabled when empty")
}

func runDeploy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	tui.SetRunID(uuid.NewString())

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

	if deployYes {
		return saveConfig(cfg, deployOutputFile)
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
		return runFullDeployment(ctx, cfg)
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

	if err := saveConfig(cfg, deployOutputFile); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	switch result.Action {
	case wizard.ActionDeploy:
		if err := runFullDeployment(ctx, cfg); err != nil {
			return fmt.Errorf("deployment failed: %w", err)
		}
	case wizard.ActionExit:
		showExitSummary(deployOutputFile)
	}

	return nil
}

// runDeployDryRun previews a deploy: runs terraform plan and lists every phase
// step. Requires terraform.tfvars from a prior setup run; absent tfvars causes
// plan failure and exits 2.
func runDeployDryRun(ctx context.Context, cfg *config.Config) error {
	envPath := credentials.EnvFilePath(deployOutputFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		tui.Warn("failed to load credentials", tui.LF("path", envPath), tui.LF("err", err))
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	defer creds.Zeroize()

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	prov := proxmox.New(
		proxmox.WithProjectRoot(projectRoot),
		proxmox.WithLogger(tui.SimpleLogger()),
		proxmox.WithEnv(creds.Env()),
	)
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

	fmt.Println(DryRunSummary("deploy step listing", deployDryRunSteps()))
	tui.Info("dry-run: re-run without --dry-run to execute deploy")
	return nil
}

// deployDryRunSteps returns the ID/Name for every step across setup, install, and
// postinstall phases in execution order.
func deployDryRunSteps() []DryRunStep {
	return []DryRunStep{
		{ID: "install-packages", Name: "install system packages"},
		{ID: "install-tools", Name: "install external tools"},
		{ID: "ensure-workdir", Name: "ensure work directory"},
		{ID: "download-tools", Name: "download okd tools"},
		{ID: "generate-config", Name: "generate install config"},
		{ID: "generate-manifests", Name: "generate manifests"},
		{ID: "generate-kubevip-manifests", Name: "generate kube-vip manifests"},
		{ID: "inject-manifests", Name: "inject custom manifests"},
		{ID: "compact-cluster-manifests", Name: "inject compact cluster manifests"},
		{ID: "generate-ignition", Name: "generate ignition"},
		{ID: "install-apache", Name: "install apache"},
		{ID: "deploy-ignition", Name: "deploy ignition"},
		{ID: "verify-webserver", Name: "verify web server"},
		{ID: "build-isos", Name: "build isos"},
		{ID: "upload-isos", Name: "upload isos"},
		{ID: "generate-tfvars", Name: "generate terraform variables"},
		{ID: "configure-haproxy", Name: "configure haproxy"},
		{ID: "configure-firewall", Name: "configure firewall"},
		{ID: "configure-dns", Name: "configure dns"},
		{ID: "deploy-infrastructure", Name: "deploy infrastructure"},
		{ID: "wait-bootstrap", Name: "wait for bootstrap"},
		{ID: "start-workers", Name: "start worker nodes"},
		{ID: "setup-kubeconfig", Name: "setup kubeconfig"},
		{ID: "validate-access", Name: "validate cluster access"},
		{ID: "monitor-install", Name: "monitor installation"},
		{ID: "setup-access", Name: "setup cluster access"},
		{ID: "verify-health", Name: "verify cluster health"},
		{ID: "cleanup-bootstrap", Name: "cleanup bootstrap vm"},
		{ID: "verify-kubevip", Name: "verify kube-vip"},
		{ID: "deploy-production-dns", Name: "deploy production dns"},
		{ID: "install-addons", Name: "install addons"},
	}
}

func saveConfig(cfg *config.Config, path string) error {
	if result := validateConfig(cfg); !result.IsValid() {
		tui.Warn("configuration has validation warnings but will still be saved")
	}

	loader := config.NewLoader()
	if err := loader.Save(cfg, path); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	return nil
}

func runFullDeployment(ctx context.Context, cfg *config.Config) error {
	if deployDryRun {
		return runDeployDryRun(ctx, cfg)
	}

	envPath := credentials.EnvFilePath(deployOutputFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		tui.Warn("failed to load credentials", tui.LF("path", envPath), tui.LF("err", err))
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	defer creds.Zeroize()

	if !creds.IsValid() {
		tui.Warn("no proxmox credentials found")
	} else {
		tui.Info(fmt.Sprintf("using credentials from %s", creds.Source))
		if creds.ConfigCredentialsOverridden {
			tui.Warn("environment credentials override proxmox credentials in config file")
		}
		if creds.EndpointFromConfig {
			tui.Warn("PROXMOX_VE_ENDPOINT not set; endpoint falling back to config file (mixed source)")
		}
	}

	return executeFullDeployment(ctx, cfg, deploymentOptions{
		ShowStartMessage: true,
		Credentials:      creds,
		MetricsAddr:      deployMetricsAddr,
	})
}

func showExitSummary(path string) {
	fmt.Println()
	tui.Info(fmt.Sprintf("configuration saved to %s", path))
}

func writeCredentialsEnv(cfg *config.Config, configPath string) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}
	px := cfg.Provider.Proxmox

	if px.Password == "" && px.APIToken == "" {
		return nil
	}

	// Resolve the normalized endpoint (adds https:// and :8006 as needed)
	// so the .env file is self-contained for Proxmox connection.
	resolved := credentials.GetProxmoxCredentials(cfg)

	creds := &credentials.ProxmoxCredentials{
		Endpoint: resolved.Endpoint,
		Username: px.Username,
		Password: []byte(px.Password),
		APIToken: []byte(px.APIToken),
		Insecure: px.Insecure,
	}
	defer creds.Zeroize()

	envPath := credentials.EnvFilePath(configPath)
	if err := credentials.WriteEnvFile(envPath, creds); err != nil {
		return err
	}

	tui.Info(fmt.Sprintf("credentials saved to %s", envPath))
	return nil
}

// clearConfigCredentials removes secrets so they are never serialized to YAML.
func clearConfigCredentials(cfg *config.Config) {
	if cfg.Provider.Proxmox == nil {
		return
	}
	cfg.Provider.Proxmox.Password = ""
	cfg.Provider.Proxmox.APIToken = ""
}
