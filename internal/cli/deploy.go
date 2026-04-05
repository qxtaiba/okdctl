package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/credentials"
	"github.com/qxtaiba/okd-proxmox-cli/internal/deployment"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/steps"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

var (
	deployOutputFile     string
	deployMinimal        bool
	deployNonInteractive bool
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a Kubernetes cluster",
	Long:  `Deploy an OKD/OpenShift cluster through an interactive wizard.`,
	RunE:  runDeploy,
}

func init() {
	deployCmd.Flags().StringVarP(&deployOutputFile, "output", "o", "openshitctl.yaml", "output file for configuration")
	deployCmd.Flags().BoolVar(&deployMinimal, "minimal", false, "use minimal defaults (single-node cluster)")
	deployCmd.Flags().BoolVar(&deployNonInteractive, "non-interactive", false, "use all defaults without prompts")
}

func runDeploy(cmd *cobra.Command, args []string) error {
	configExists := false
	var cfg *config.Config

	if _, err := os.Stat(deployOutputFile); err == nil {
		configExists = true
		loader := config.NewLoader()
		loadedCfg, loadErr := loader.LoadFile(deployOutputFile)
		if loadErr != nil {
			tui.Warn("existing config could not be loaded, starting fresh")
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

	if deployNonInteractive {
		return saveConfig(cfg, deployOutputFile)
	}

	result, welcomeMode, err := runWizardWithMode(cfg, configExists)
	if err != nil {
		return utils.WrapError("wizard failed", err)
	}

	if result.Cancelled {
		tui.Info("wizard cancelled, no changes made")
		return nil
	}

	if welcomeMode == steps.WelcomeModeDeploy {
		return runFullDeployment(cfg)
	}

	cfg = result.Config

	if err := writeCredentialsEnv(cfg, deployOutputFile); err != nil {
		return utils.WrapError("failed to save credentials", err)
	}

	clearConfigCredentials(cfg)

	if err := saveConfig(cfg, deployOutputFile); err != nil {
		return utils.WrapError("failed to save configuration", err)
	}

	switch result.Action {
	case wizard.ActionDeploy:
		if err := runFullDeployment(cfg); err != nil {
			return utils.WrapError("deployment failed", err)
		}
	case wizard.ActionExit:
		showExitSummary(deployOutputFile)
	}

	return nil
}

func saveConfig(cfg *config.Config, path string) error {
	if result := ValidateConfig(cfg); !result.IsValid() {
		tui.Warn("configuration has validation warnings but will still be saved")
	}

	loader := config.NewLoader()
	if err := loader.Save(cfg, path); err != nil {
		return utils.WrapError("failed to save configuration", err)
	}

	return nil
}

func runFullDeployment(cfg *config.Config) error {
	handler, ctx := deployment.NewInterruptHandler()
	defer handler.Cleanup()

	handler.OnInterrupt = func(sig os.Signal) {
		fmt.Println()
		tui.Error(fmt.Sprintf("deployment interrupted by %v", sig))
		tui.Info("run 'openshitctl destroy' to clean up resources")
	}

	creds := HandleCredentials(cfg)
	defer creds.Zeroize()

	return ExecuteFullDeployment(ctx, cfg, DeploymentOptions{
		ShowStartMessage: true,
		Credentials:      creds,
	})
}

func showExitSummary(path string) {
	fmt.Println()
	tui.Info("configuration saved to " + path)
}

func writeCredentialsEnv(cfg *config.Config, configPath string) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}
	px := cfg.Provider.Proxmox

	if px.Password == "" && px.APIToken == "" {
		return nil
	}

	creds := &credentials.ProxmoxCredentials{
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

	tui.Info("credentials saved to " + envPath)
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
