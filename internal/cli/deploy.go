package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
)

var (
	deployOutputFile     string
	deployMinimal        bool
	deployNonInteractive bool
	deployYes            bool
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
	deployCmd.Flags().BoolVar(&deployNonInteractive, "non-interactive", false, "use all defaults without prompts")
	deployCmd.Flags().BoolVarP(&deployYes, "yes", "y", false, "skip prompts, use defaults (alias for --non-interactive)")
}

func runDeploy(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if deployYes {
		deployNonInteractive = true
	}
	configExists := false
	var cfg *config.Config

	if _, err := os.Stat(deployOutputFile); err == nil {
		configExists = true
		loader := config.NewLoader()
		loadedCfg, loadErr := loader.LoadFile(deployOutputFile)
		if loadErr != nil {
			tui.Warn(fmt.Sprintf("existing config could not be loaded: %v", loadErr))
			if deployNonInteractive {
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

	if deployNonInteractive {
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
	envPath := credentials.EnvFilePath(deployOutputFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		tui.Warn(fmt.Sprintf("failed to load credentials from %s: %v", envPath, err))
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
