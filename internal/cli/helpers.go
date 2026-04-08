package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/credentials"
	"github.com/qxtaiba/okd-proxmox-cli/internal/deployment"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

func LoadConfig(configFile string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			tui.Error("configuration file not found: " + configFile)
			if configFile == "openshitctl.yaml" {
				tui.Info("run 'openshitctl deploy' to create a configuration file")
			} else {
				tui.Info("run 'openshitctl deploy --output " + configFile + "' to create it")
			}
			return nil, fmt.Errorf("configuration file not found: %s: %w", configFile, err)
		}
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}

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
		if creds.ConfigCredentialsOverridden {
			tui.Warn("environment credentials override proxmox credentials in config file")
		}
		if creds.EndpointFromConfig {
			tui.Warn("PROXMOX_VE_ENDPOINT not set; endpoint falling back to config file (mixed source)")
		}
	}
	return creds
}

func ValidateConfig(cfg *config.Config) *config.ValidationResult {
	result := cfg.Validate()
	if !result.IsValid() {
		fmt.Println(ValidationSummary(result))
	}
	return result
}

// resolveProjectRoot returns a canonical absolute path for the current working
// directory. filepath.Abs makes intent explicit even though Getwd already
// returns an absolute path; EvalSymlinks canonicalizes it.
func resolveProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Symlink resolution can fail harmlessly (e.g., temp dirs on macOS).
		// Fall back to the absolute path.
		return abs, nil
	}
	return resolved, nil
}

// projectRootOrFallback resolves the project root and logs a debug message on
// failure, returning an empty string so callers can decide how to proceed.
func projectRootOrFallback() string {
	root, err := resolveProjectRoot()
	if err != nil {
		tui.Debug("failed to resolve project root: " + err.Error())
		return ""
	}
	return root
}

// CreateOKDProvisioner creates a provisioner, optionally with Proxmox credentials.
// Pass nil for creds when the operation only needs local tools (oc, dnsmasq, systemctl).
func CreateOKDProvisioner(cfg *config.Config, creds *credentials.ProxmoxCredentials) *okd.Provisioner {
	projectRoot := projectRootOrFallback()

	opts := []okd.ProvisionerOption{
		okd.WithProjectRoot(projectRoot),
		okd.WithLogger(tui.SimpleLogger()),
	}

	if creds != nil && creds.IsValid() {
		opts = append(opts, okd.WithEnv(creds.Env()))
	}

	return okd.New(cfg.Distribution.Version, opts...)
}

type DeploymentOptions struct {
	ShowStartMessage bool
	Credentials      *credentials.ProxmoxCredentials
}

func ExecuteFullDeployment(ctx context.Context, cfg *config.Config, opts DeploymentOptions) error {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain

	var credsEnv []string
	if opts.Credentials != nil && opts.Credentials.IsValid() {
		credsEnv = opts.Credentials.Env()
	}

	return deployment.Run(ctx, cfg, deployment.Options{
		ShowStartMessage: opts.ShowStartMessage,
		Logger:           tui.SimpleLogger(),
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
