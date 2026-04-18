package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func loadConfig(configFile string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			tui.Error(fmt.Sprintf("configuration file not found: %s", configFile))
			if configFile == "okdctl.yaml" {
				tui.Info("run 'okdctl deploy' to create a configuration file")
			} else {
				tui.Info(fmt.Sprintf("run 'okdctl deploy --output %s' to create it", configFile))
			}
			return nil, fmt.Errorf("configuration file not found: %s: %w", configFile, err)
		}
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}

// Loads .env first (non-overwriting) so env vars always win.
func handleCredentials(cfg *config.Config) *credentials.ProxmoxCredentials {
	envPath := credentials.EnvFilePath(cfgFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		tui.Warn(fmt.Sprintf("failed to load credentials from %s: %v", envPath, err))
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	if !creds.IsValid() {
		tui.Warn("no proxmox credentials found")
		tui.Info(fmt.Sprintf("set credentials via environment variables or %s:", envPath))
		tui.Info("  PROXMOX_VE_USERNAME + PROXMOX_VE_PASSWORD")
		tui.Info("  or PROXMOX_VE_API_TOKEN")
	} else {
		tui.Info(fmt.Sprintf("using credentials from %s", creds.Source))
		if creds.ConfigCredentialsOverridden {
			tui.Warn("environment credentials override proxmox credentials in config file")
		}
		if creds.EndpointFromConfig {
			tui.Warn("PROXMOX_VE_ENDPOINT not set; endpoint falling back to config file (mixed source)")
		}
	}
	return creds
}

func validateConfig(cfg *config.Config) *config.ValidationResult {
	result := cfg.Validate()
	if !result.IsValid() {
		fmt.Println(ValidationSummary(result))
	}
	return result
}

// resolveProjectRoot returns the canonical absolute path of the current
// working directory.
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
		return abs, nil //nolint:nilerr // intentional fallback to abs path when symlink resolution fails
	}
	return resolved, nil
}

// resolveProjectRootOrDie resolves the project root or returns an error.
func resolveProjectRootOrDie() (string, error) {
	root, err := resolveProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to resolve project root: %w", err)
	}
	if root == "" {
		return "", fmt.Errorf("project root resolved to empty path")
	}
	return root, nil
}

// CreateOKDProvisioner creates a provisioner, optionally with Proxmox credentials.
// Pass nil for creds when the operation only needs local tools (oc, dnsmasq, systemctl).
func createOKDProvisioner(cfg *config.Config, creds *credentials.ProxmoxCredentials, projectRoot string) *okd.Provisioner {
	opts := []okd.ProvisionerOption{
		okd.WithProjectRoot(projectRoot),
		okd.WithLogger(tui.SimpleLogger()),
	}

	if creds != nil && creds.IsValid() {
		opts = append(opts, okd.WithEnv(creds.Env()))
	}

	return okd.New(cfg.Distribution.Version, opts...)
}

type deploymentOptions struct {
	ShowStartMessage bool
	Credentials      *credentials.ProxmoxCredentials
}

func executeFullDeployment(ctx context.Context, cfg *config.Config, opts deploymentOptions) error {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	// Deploy writes per-run artifacts (install-config.yaml, manifests,
	// ignition files, downloaded tools, ISOs) under <projectRoot>/okd-install.
	// Under the sudo re-exec model these are root-owned by default; restore
	// ownership to the invoking user at exit so they can inspect and rm -rf
	// the workdir without sudo. No-op when not running under sudo.
	//
	// If resolveProjectRootOrDie failed above, we returned early — the
	// workdir cannot exist yet because no phase code has run.
	workDir := filepath.Join(projectRoot, "okd-install")
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			tui.Warn(fmt.Sprintf("workdir chown back to user incomplete: %v", chownErr))
		}
	}()

	p := createOKDProvisioner(cfg, opts.Credentials, projectRoot)

	if err := p.Validate(cfg); err != nil {
		return fmt.Errorf("provisioner validation failed: %w", err)
	}

	if opts.ShowStartMessage {
		tui.Info("starting deployment...", tui.LF("cluster", clusterFQDN))
	}

	startTime := time.Now()

	setupSteps, err := p.Prepare(ctx, cfg)
	if err != nil {
		tui.Info("run 'okdctl destroy' to clean up resources")
		return fmt.Errorf("deployment failed: %w", err)
	}

	installOpts := install.NewOptions(cfg, projectRoot)
	installSteps, err := p.Install(ctx, cfg, &installOpts)
	if err != nil {
		tui.Info("run 'okdctl destroy' to clean up resources")
		return fmt.Errorf("deployment failed: %w", err)
	}

	result, configureSteps, err := p.Configure(ctx, cfg)
	if err != nil {
		tui.Info("run 'okdctl destroy' to clean up resources")
		return fmt.Errorf("deployment failed: %w", err)
	}

	allSteps := make([]distribution.StepResult, 0, len(setupSteps)+len(installSteps)+len(configureSteps))
	allSteps = append(allSteps, setupSteps...)
	allSteps = append(allSteps, installSteps...)
	allSteps = append(allSteps, configureSteps...)

	duration := time.Since(startTime).Round(time.Second)

	fmt.Println()
	tui.Info(fmt.Sprintf("deployment complete (total time: %s)", duration))
	fmt.Println(PostDeploySummary(cfg, result, allSteps))

	return nil
}
