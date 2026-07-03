package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/deploymetrics"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const deployStateFile = ".okdctl-deploy-state.json"

func loadConfig(configFile string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			tui.Error("configuration file not found", tui.LF("file", configFile))
			if configFile == "okdctl.yaml" {
				tui.Info("run 'okdctl deploy' to create a configuration file")
			} else {
				tui.Info("run 'okdctl deploy --output-file <file>' to create it", tui.LF("file", configFile))
			}
			return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("configuration file not found: %s", configFile), Err: errtypes.ErrConfigMissing}
		}
		return nil, &errtypes.ConfigError{Msg: "load configuration", Err: err}
	}
	return cfg, nil
}

// handleCredentials loads the .env file (non-overwriting) then resolves
// Proxmox credentials from environment and config. Returns an error if the
// .env could not be loaded — callers must not proceed in that case.
func handleCredentials(cfg *config.Config) (*credentials.ProxmoxCredentials, error) {
	envPath := credentials.EnvFilePath(cfgFile)
	if err := credentials.LoadEnvFile(envPath); err != nil {
		return nil, err
	}

	creds := credentials.GetProxmoxCredentials(cfg)
	if !creds.IsValid() {
		tui.Warn("no proxmox credentials found")
		tui.Info("set credentials via environment variables or env file", tui.LF("path", envPath))
		tui.Info("  PROXMOX_VE_USERNAME + PROXMOX_VE_PASSWORD")
		tui.Info("  or PROXMOX_VE_API_TOKEN")
	} else {
		reportCredentialProvenance(creds)
	}
	return creds, nil
}

// reportCredentialProvenance logs the resolved credential source plus any
// mixed-provenance warnings the operator should see (env-overrides-config,
// endpoint falling back to config). Callers must check creds.IsValid first.
func reportCredentialProvenance(creds *credentials.ProxmoxCredentials) {
	tui.Info("using credentials", tui.LF("source", creds.Source))
	if creds.ConfigCredentialsOverridden {
		tui.Warn("environment credentials override proxmox credentials in config file")
	}
	if creds.EndpointFromConfig {
		tui.Warn("PROXMOX_VE_ENDPOINT not set; endpoint falling back to config file (mixed source)")
	}
	if creds.Insecure {
		tui.Warn("proxmox: TLS verification disabled (insecure=true in config)")
	}
}

func validateConfig(cfg *config.Config, w io.Writer) *config.ValidationResult {
	result := cfg.Validate()
	if !result.IsValid() {
		fmt.Fprintln(w, ValidationSummary(result))
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
	// Project marker check: okdctl.yaml and okdctl.env are the primary
	// markers. terraform.tfstate is a secondary recovery hint — it can
	// outlive a successful destroy+cleanup, which preserves it for
	// resumability. At least one of the three must be present; a lone
	// tfstate triggers an explicit operator warning (warnIfTfStateOnly)
	// because it may belong to a different cluster.
	if !hasProjectMarker(root) {
		return "", &errtypes.ConfigError{
			Msg: fmt.Sprintf(
				"no project marker found in %s "+
					"(checked okdctl.yaml, okdctl.env, "+
					"infrastructure/terraform/environments/*/terraform.tfstate); "+
					"run 'okdctl deploy' to initialise",
				root,
			),
			Err: errtypes.ErrConfigMissing,
		}
	}
	warnIfTfStateOnly(root)
	return root, nil
}

// hasProjectMarker reports whether root contains at least one okdctl project
// file. It checks the configured config-file name, okdctl.env, and any
// terraform.tfstate under infrastructure/terraform/environments/. All three
// are exclusively written by okdctl inside a project root.
func hasPrimaryMarker(root string) bool {
	for _, name := range []string{filepath.Base(cfgFile), "okdctl.env"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

func terraformStateMatches(root string) []string {
	matches, _ := filepath.Glob(
		filepath.Join(root, "infrastructure", "terraform", "environments", "*", "terraform.tfstate"),
	)
	return matches
}

func hasProjectMarker(root string) bool {
	return hasPrimaryMarker(root) || len(terraformStateMatches(root)) > 0
}

// warnIfTfStateOnly emits a structured warning when the only project marker
// is terraform.tfstate (okdctl.yaml and okdctl.env both absent). A lone
// tfstate after destroy+cleanup may belong to a different cluster if the
// operator removed the primary config files manually.
func warnIfTfStateOnly(root string) {
	if hasPrimaryMarker(root) {
		return
	}
	matches := terraformStateMatches(root)
	if len(matches) == 0 {
		return
	}
	tui.Warn("okdctl.yaml and okdctl.env not found; accepting terraform.tfstate as a recovery hint",
		tui.LF("tfstate", matches[0]),
		tui.LF("root", root),
	)
	tui.Info("if this directory belongs to a different cluster, stop and run 'okdctl deploy' in the correct directory")
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

// runGuardedPrepare runs the prepare phase behind the live-cluster guard.
// The deploy-state marker is read BEFORE it is overwritten (a present marker
// means the prior run was interrupted, so the guard must let the documented
// re-run-to-resume flow proceed) and the guard probe runs before any marker
// write — a refusal must not plant a marker that would bypass the guard on
// the next invocation.
func runGuardedPrepare(ctx context.Context, p *okd.Provisioner, cfg *config.Config, markerPath, runID string, freshDeploy bool, w io.Writer) ([]distribution.StepResult, error) {
	existingMarker, _ := readDeployState(markerPath)
	prepOpts := okd.PrepareOpts{FreshDeploy: freshDeploy, ResumeInProgress: existingMarker != nil && !freshDeploy}
	if err := p.GuardPrepare(cfg, prepOpts); err != nil {
		return nil, err
	}

	if err := markDeployPhaseFatal(markerPath, phasePrepare, runID, cfg.Cluster.Name); err != nil {
		return nil, err
	}
	setupSteps, err := p.Prepare(ctx, cfg, prepOpts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(w, InterruptSummary(setupSteps, "okdctl deploy", runID))
			tui.Info("cancelled during prepare — terraform state is empty; run 'okdctl cleanup' to remove local files")
			return setupSteps, err
		}
		// prepare applies nothing to Proxmox; destroy would be a misleading no-op.
		tui.Info("prepare failed — terraform state is empty; run 'okdctl cleanup' to remove local files")
		return setupSteps, err
	}
	return setupSteps, nil
}

type deploymentOptions struct {
	ShowStartMessage    bool
	Credentials         *credentials.ProxmoxCredentials
	MetricsAddr         string
	AllowNetworkMetrics bool
	FreshDeploy         bool
}

// metricsReadHeaderTimeout / metricsReadTimeout / metricsWriteTimeout set
// conservative HTTP server bounds for the unauthenticated /metrics endpoint.
// metricsIdleTimeout leaves slack for Prometheus scrapers that reconnect on
// their configured scrape_interval (typically 15–60 s); metricsShutdownTimeout
// gives in-flight scrapes time to drain on graceful stop.
const (
	metricsReadHeaderTimeout = 5 * time.Second
	metricsReadTimeout       = 10 * time.Second
	metricsWriteTimeout      = 10 * time.Second
	metricsIdleTimeout       = 60 * time.Second
	metricsShutdownTimeout   = 5 * time.Second
)

// startMetricsServer starts a Prometheus metrics HTTP server on addr (disabled
// when addr is empty). Returns a stop closure that shuts the server down with
// a 5-second deadline and surfaces any bind error, plus any provisioner
// options the caller must apply so orchestrated phases feed observations to
// the recorder.
//
// Bare ":port" is rewritten to "127.0.0.1:port" so the unauthenticated
// listener does not leak to the network by default. Wildcard addresses
// (0.0.0.0 or [::]) are rejected unless allowNetwork is true; pass
// --metrics-allow-network to opt in.
func startMetricsServer(ctx context.Context, addr string, allowNetwork bool) (func() error, []okd.ProvisionerOption, error) {
	if addr == "" {
		return func() error { return nil }, nil, nil
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, nil, &errtypes.ConfigError{Msg: fmt.Sprintf("invalid --metrics-addr %q", addr), Err: err}
	}
	if host != "" {
		parsed, parseErr := netip.ParseAddr(host)
		if parseErr == nil && parsed.IsUnspecified() && !allowNetwork {
			return nil, nil, &errtypes.ConfigError{Msg: "wildcard metrics bind requires --metrics-allow-network"}
		}
	}
	rec := deploymetrics.NewRecorder()
	mux := http.NewServeMux()
	mux.Handle("/metrics", rec.Handler())
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// BaseContext propagates the deploy ctx so in-flight scrape connections
		// are cancelled when the parent context is cancelled.
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: metricsReadHeaderTimeout,
		ReadTimeout:       metricsReadTimeout,
		WriteTimeout:      metricsWriteTimeout,
		IdleTimeout:       metricsIdleTimeout,
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, &errtypes.ConfigError{Msg: fmt.Sprintf("metrics bind failed on %q", addr), Err: err}
	}
	tui.Info("metrics endpoint listening", tui.LF("addr", addr))
	// errCh cap=1: the goroutine sends exactly once and never blocks, so it
	// exits cleanly even if stop is never called (early return on phase error).
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	stop := func() error {
		// Use Background, not the caller's ctx: by stop() time the parent ctx
		// is already cancelled by SIGINT, and we need the 5s drain to complete.
		shutCtx, cancel := context.WithTimeout(context.Background(), metricsShutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		// Shutdown's return guarantees ListenAndServe has exited; drain errCh.
		select {
		case err := <-errCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		default:
			return nil
		}
	}
	return stop, []okd.ProvisionerOption{okd.WithMetricsRecorder(rec)}, nil
}

func executeFullDeployment(ctx context.Context, cfg *config.Config, opts deploymentOptions, w io.Writer) error {
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

	stopMetrics, provOpts, err := startMetricsServer(ctx, opts.MetricsAddr, opts.AllowNetworkMetrics)
	if err != nil {
		return err
	}
	defer func() {
		if stopErr := stopMetrics(); stopErr != nil {
			tui.Warn("metrics server stopped with error", tui.LF("err", stopErr))
		}
	}()

	provOpts = append(provOpts, okd.WithProgressReporter(func(desc string) func() { return tui.StartSpinner(ctx, desc) }))
	p := createOKDProvisionerWithOpts(cfg, opts.Credentials, projectRoot, provOpts...)
	defer p.ZeroizeEnv()

	if err := p.Validate(cfg); err != nil {
		return fmt.Errorf("provisioner validation failed: %w", err)
	}

	if opts.ShowStartMessage {
		tui.Info("starting deployment...", tui.LF("cluster", cfg.Cluster.Name+"."+cfg.Cluster.Domain))
	}

	startTime := time.Now()
	markerPath := filepath.Join(workDir, deployStateFile)

	setupSteps, err := runGuardedPrepare(ctx, p, cfg, markerPath, runID, opts.FreshDeploy, w)
	if err != nil {
		return err
	}

	markDeployPhase(markerPath, phaseInstall, runID, cfg.Cluster.Name)
	installOpts := install.NewOptions(cfg, projectRoot)
	installSteps, err := p.Install(ctx, cfg, &installOpts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			combined := slices.Concat(setupSteps, installSteps)
			fmt.Fprintln(w, InterruptSummary(combined, "okdctl deploy", runID))
			tui.Info("cancelled during install — terraform state likely populated; run 'okdctl destroy' to clean up")
			return err
		}
		tui.Info("install failed — terraform state likely populated; run 'okdctl destroy' to clean up")
		return err
	}

	markDeployPhase(markerPath, phaseConfigure, runID, cfg.Cluster.Name)
	result, configureSteps, err := p.Configure(ctx, cfg)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			combined := slices.Concat(setupSteps, installSteps, configureSteps)
			fmt.Fprintln(w, InterruptSummary(combined, "okdctl deploy", runID))
			tui.Info("cancelled during configure — terraform state likely populated; run 'okdctl destroy' to clean up")
			return err
		}
		tui.Info("configure failed — terraform state likely populated; run 'okdctl destroy' to clean up")
		return err
	}

	allSteps := slices.Concat(setupSteps, installSteps, configureSteps)
	clearDeployMarker(markerPath)

	duration := time.Since(startTime).Round(time.Second)

	fmt.Fprintln(w)
	tui.Info("deployment complete", tui.LF("duration", duration))
	fmt.Fprintln(w, PostDeploySummary(cfg, result, allSteps, runID))

	return nil
}
