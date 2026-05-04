package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/deploymetrics"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

func loadConfig(configFile string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			tui.Error(fmt.Sprintf("configuration file not found: %s", configFile))
			if configFile == "okdctl.yaml" {
				tui.Info("run 'okdctl deploy' to create a configuration file")
			} else {
				tui.Info(fmt.Sprintf("run 'okdctl deploy --output %s' to create it", configFile))
			}
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("configuration file not found: %s", configFile), Err: errtypes.ErrConfigMissing}
		}
		return nil, &errtypes.ConfigError{Msg: "load configuration", Err: err}
	}
	return cfg, nil
}

// Loads .env first (non-overwriting) so env vars always win.
func handleCredentials(cfg *config.Config) *credentials.ProxmoxCredentials {
	envPath := credentials.EnvFilePath(cfgFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		tui.Warn("failed to load credentials", tui.LF("path", envPath), tui.LF("err", err))
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	if !creds.IsValid() {
		tui.Warn("no proxmox credentials found")
		tui.Info(fmt.Sprintf("set credentials via environment variables or %s:", envPath))
		tui.Info("  PROXMOX_VE_USERNAME + PROXMOX_VE_PASSWORD")
		tui.Info("  or PROXMOX_VE_API_TOKEN")
	} else {
		reportCredentialProvenance(creds)
	}
	return creds
}

// reportCredentialProvenance logs the resolved credential source plus any
// mixed-provenance warnings the operator should see (env-overrides-config,
// endpoint falling back to config). Callers must check creds.IsValid first.
func reportCredentialProvenance(creds *credentials.ProxmoxCredentials) {
	tui.Info(fmt.Sprintf("using credentials from %s", creds.Source))
	if creds.ConfigCredentialsOverridden {
		tui.Warn("environment credentials override proxmox credentials in config file")
	}
	if creds.EndpointFromConfig {
		tui.Warn("PROXMOX_VE_ENDPOINT not set; endpoint falling back to config file (mixed source)")
	}
}

func validateConfig(cfg *config.Config) *config.ValidationResult {
	result := cfg.Validate()
	if !result.IsValid() {
		fmt.Println(ValidationSummary(result))
	}
	return result
}

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
		// EvalSymlinks fails with ErrNotExist when a path component does not
		// exist yet (e.g., macOS temp dirs on startup). That case is benign:
		// fall back to the absolute path. Any other error — EPERM, EIO, etc.
		// — is a real filesystem problem that must not be silently swallowed,
		// because abs may still be a symlink and handing it to sudo-elevated
		// helpers would let an attacker redirect writes to arbitrary paths.
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("resolve project root symlinks: %w", err)
		}
		return abs, nil
	}
	return resolved, nil
}

func resolveProjectRootOrDie() (string, error) {
	root, err := resolveProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to resolve project root: %w", err)
	}
	if root == "" {
		return "", fmt.Errorf("project root resolved to empty path")
	}
	// Project marker check: refuse if the resolved root does not contain the
	// configured config file. This stops a symlink that resolves outside the
	// project from redirecting sudo-elevated cleanup at arbitrary paths.
	// filepath.Base ensures the check stays inside root even when --config is
	// an absolute path.
	marker := filepath.Join(root, filepath.Base(cfgFile))
	if _, statErr := os.Stat(marker); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return "", &errtypes.ConfigError{
				Msg: fmt.Sprintf("project marker not found at %s; run 'okdctl deploy' to initialise", marker),
				Err: errtypes.ErrConfigMissing,
			}
		}
		return "", fmt.Errorf("stat project marker %s: %w", marker, statErr)
	}
	return root, nil
}

// Pass nil for creds when the operation only needs local tools (oc, dnsmasq, systemctl).
func createOKDProvisionerWithOpts(cfg *config.Config, creds *credentials.ProxmoxCredentials, projectRoot string, extra ...okd.ProvisionerOption) *okd.Provisioner {
	opts := []okd.ProvisionerOption{
		okd.WithProjectRoot(projectRoot),
		okd.WithLogger(tui.SimpleLogger()),
	}

	if creds != nil && creds.IsValid() {
		opts = append(opts, okd.WithEnv(creds.Env()))
	}

	opts = append(opts, extra...)
	return okd.New(cfg.Distribution.Version, opts...)
}

type deploymentOptions struct {
	ShowStartMessage bool
	Credentials      *credentials.ProxmoxCredentials
	MetricsAddr      string
}

// startMetricsServer starts a Prometheus metrics HTTP server on addr (disabled
// when addr is empty). Returns a stop closure that shuts the server down with
// a 5-second deadline, plus any provisioner options the caller must apply so
// orchestrated phases feed observations to the recorder.
//
// The bare ":port" shorthand binds every interface; we rewrite it to
// "127.0.0.1:port" by default so an unauth listener does not leak to the
// network. Operators who explicitly want a wildcard bind can pass
// "0.0.0.0:port".
func startMetricsServer(addr string) (func(), []okd.ProvisionerOption) {
	if addr == "" {
		return func() {}, nil
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	rec := deploymetrics.NewRecorder()
	mux := http.NewServeMux()
	mux.Handle("/metrics", rec.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.ListenAndServe() }()
	tui.Info("metrics endpoint listening", tui.LF("addr", addr))
	stop := func() {
		// Use Background, not the caller's ctx: by stop() time the parent ctx
		// is already cancelled by SIGINT, and we need the 5s drain to complete.
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}
	return stop, []okd.ProvisionerOption{okd.WithMetricsRecorder(rec)}
}

func executeFullDeployment(ctx context.Context, cfg *config.Config, opts deploymentOptions) error {
	clusterFQDN := cfg.Cluster.Name + "." + cfg.Cluster.Domain
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	lock, err := runlock.Acquire(projectRoot, "deploy")
	if err != nil {
		return err
	}
	defer lock.Release()

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
			tui.Warn("workdir chown back to user incomplete", tui.LF("err", chownErr))
		}
	}()

	runID := tui.RunID()

	stopMetrics, provOpts := startMetricsServer(opts.MetricsAddr)
	defer stopMetrics()

	p := createOKDProvisionerWithOpts(cfg, opts.Credentials, projectRoot, provOpts...)

	if err := p.Validate(cfg); err != nil {
		return fmt.Errorf("provisioner validation failed: %w", err)
	}

	if opts.ShowStartMessage {
		tui.Info("starting deployment...", tui.LF("cluster", clusterFQDN))
	}

	startTime := time.Now()

	setupSteps, err := p.Prepare(ctx, cfg)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println(InterruptSummary(setupSteps, "okdctl deploy", runID))
			return err
		}
		tui.Info("run 'okdctl destroy' to clean up resources")
		return err
	}

	installOpts := install.NewOptions(cfg, projectRoot)
	installSteps, err := p.Install(ctx, cfg, &installOpts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			combined := slices.Concat(setupSteps, installSteps)
			fmt.Println(InterruptSummary(combined, "okdctl deploy", runID))
			return err
		}
		tui.Info("run 'okdctl destroy' to clean up resources")
		return err
	}

	result, configureSteps, err := p.Configure(ctx, cfg)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			combined := slices.Concat(setupSteps, installSteps, configureSteps)
			fmt.Println(InterruptSummary(combined, "okdctl deploy", runID))
			return err
		}
		tui.Info("run 'okdctl destroy' to clean up resources")
		return err
	}

	allSteps := slices.Concat(setupSteps, installSteps, configureSteps)

	duration := time.Since(startTime).Round(time.Second)

	fmt.Println()
	tui.Info(fmt.Sprintf("deployment complete (total time: %s)", duration))
	fmt.Println(PostDeploySummary(cfg, result, allSteps, runID))

	return nil
}
