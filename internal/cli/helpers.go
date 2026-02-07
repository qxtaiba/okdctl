// Package cli provides CLI command implementations.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/credentials"
	"github.com/qxtaiba/okd-proxmox-cli/internal/deployment"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

var ErrConfigNotFound = errors.New("configuration file not found")

// LoadConfig loads a configuration file with helpful error messages.
func LoadConfig(configFile string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			tui.Error("configuration file not found: " + configFile)
			tui.Info("run 'openshitctl init' to create a configuration file")
			return nil, ErrConfigNotFound
		}
		return nil, utils.WrapError("load configuration", err)
	}
	return cfg, nil
}

// HandleCredentials retrieves Proxmox credentials and displays status.
// Loads .env first (non-overwriting) so env vars always win.
func HandleCredentials(cfg *config.Config) *credentials.ProxmoxCredentials {
	envPath := credentials.EnvFilePath(cfgFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		tui.Warn("failed to load credentials from " + envPath + ": " + err.Error())
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	if !creds.IsValid() {
		tui.Warn("no proxmox credentials found")
		tui.Info("set credentials via environment variables or " + envPath + ":")
		tui.Info("  PROXMOX_VE_USERNAME + PROXMOX_VE_PASSWORD")
		tui.Info("  or PROXMOX_VE_API_TOKEN")
	} else {
		tui.Info("using credentials from " + creds.Source.String())
	}
	return creds
}

// ValidateConfig validates the configuration and prints any errors.
func ValidateConfig(cfg *config.Config) *config.ValidationResult {
	result := cfg.Validate()
	if !result.IsValid() {
		fmt.Println(ValidationSummary(result))
	}
	return result
}

// CreateOKDProvisionerWithCreds creates an OKD provisioner with credentials
// passed via environment to avoid modifying global process state.
func CreateOKDProvisionerWithCreds(cfg *config.Config, creds *credentials.ProxmoxCredentials) *okd.Provisioner {
	projectRoot, err := os.Getwd()
	if err != nil {
		tui.Debug("failed to get working directory, using fallback: " + err.Error())
		projectRoot = "."
	}

	opts := []okd.ProvisionerOption{
		okd.WithProjectRoot(projectRoot),
		okd.WithLogger(CLILogger()),
	}

	if creds != nil && creds.IsValid() {
		opts = append(opts, okd.WithEnv(creds.Env()))
	}

	return okd.New(cfg.Distribution.Version, opts...)
}

// CLILogger returns a Logger backed by the TUI.
func CLILogger() okd.Logger {
	return tui.SimpleLogger()
}

// DeploymentOptions configures the deployment workflow.
type DeploymentOptions struct {
	ShowStartMessage bool
	Credentials      *credentials.ProxmoxCredentials
}

// ExecuteFullDeployment runs the complete deployment workflow.
func ExecuteFullDeployment(ctx context.Context, cfg *config.Config, opts DeploymentOptions) error {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain

	var credsEnv []string
	if opts.Credentials != nil && opts.Credentials.IsValid() {
		credsEnv = opts.Credentials.Env()
	}

	return deployment.Run(ctx, cfg, deployment.Options{
		ShowStartMessage: opts.ShowStartMessage,
		Logger:           CLILogger(),
		CredentialsEnv:   credsEnv,
		OnStart: func(clusterName string) {
			tui.Info("starting deployment...", tui.LF("cluster", clusterFQDN))
		},
		OnComplete: func(duration time.Duration, result *deployment.Result) {
			fmt.Println()
			tui.Info(fmt.Sprintf("deployment complete (total time: %s)", duration))
			fmt.Println(PostDeploySummary(cfg, result))
		},
		OnError: func(err error) {
			tui.Info("run 'openshitctl destroy' to clean up resources")
		},
	})
}
