package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// StateFileName is the deploy-state marker file written under the work
// directory (<projectRoot>/okd-install).
const StateFileName = ".okdctl-deploy-state.json"

// NewProvisioner builds an okd.Provisioner wired for CLI use (tui logger,
// credential env). Pass nil for creds when the operation only needs local
// tools (oc, dnsmasq, systemctl). Callers must defer p.ZeroizeEnv().
func NewProvisioner(creds *credentials.ProxmoxCredentials, projectRoot string, extra ...okd.ProvisionerOption) *okd.Provisioner {
	opts := []okd.ProvisionerOption{
		okd.WithProjectRoot(projectRoot),
		okd.WithLogger(tui.SimpleLogger()),
	}

	if creds != nil && creds.IsValid() {
		opts = append(opts, okd.WithEnv(creds.Env()))
	}

	opts = append(opts, extra...)
	return okd.New(opts...)
}

// provisioner is the slice of okd.Provisioner the deploy flow drives.
// Tests substitute a fake to pin the resume routing — which phases run, and
// with what guard opts — without executing real phase code.
type provisioner interface {
	GuardPrepare(cfg *config.Config, opts okd.PrepareOpts) error
	Prepare(ctx context.Context, cfg *config.Config, opts okd.PrepareOpts) ([]distribution.StepResult, error)
	Install(ctx context.Context, cfg *config.Config, opts *install.Options) ([]distribution.StepResult, error)
	Configure(ctx context.Context, cfg *config.Config) (*postinstall.Result, []distribution.StepResult, error)
	ResumeConfigure(ctx context.Context, cfg *config.Config) (*postinstall.Result, []distribution.StepResult, error)
}

// runGuardedPrepare runs the prepare phase behind the live-cluster guard.
// resumeInProgress carries the caller's marker decision: only a prepare-phase
// marker for this cluster reaches here (resolveResumePhase routes install and
// configure markers past Prepare entirely), so the guard bypass cannot wipe
// material live VMs booted with. The guard probe runs before any marker
// write — a refusal must not plant a marker that would bypass the guard on
// the next invocation.
func runGuardedPrepare(ctx context.Context, p provisioner, cfg *config.Config, markerPath, runID string, freshDeploy, resumeInProgress bool, w io.Writer) ([]distribution.StepResult, error) {
	prepOpts := okd.PrepareOpts{FreshDeploy: freshDeploy, ResumeInProgress: resumeInProgress && !freshDeploy}
	if err := p.GuardPrepare(cfg, prepOpts); err != nil {
		return nil, err
	}

	if err := markDeployPhaseFatal(markerPath, phasePrepare, runID, cfg.Cluster.Name); err != nil {
		return nil, err
	}
	setupSteps, err := p.Prepare(ctx, cfg, prepOpts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(w, render.InterruptSummary(setupSteps, "okdctl deploy", runID))
			tui.Info("cancelled during prepare — terraform state is empty; run 'okdctl cleanup' to remove local files")
			return setupSteps, err
		}
		// prepare applies nothing to Proxmox; destroy would be a misleading no-op.
		tui.Info("prepare failed — terraform state is empty; run 'okdctl cleanup' to remove local files")
		return setupSteps, err
	}
	return setupSteps, nil
}

// Options configures Execute. ProjectRoot must be a resolved project root
// (see the cli package's project-marker validation).
type Options struct {
	ShowStartMessage    bool
	Credentials         *credentials.ProxmoxCredentials
	MetricsAddr         string
	AllowNetworkMetrics bool
	FreshDeploy         bool
	ProjectRoot         string
}

// runDeployPhases executes prepare, install, and configure, starting from
// the phase the deploy-state marker says is safe to resume from. Returns the
// postinstall result and every executed step across phases.
func runDeployPhases(ctx context.Context, p provisioner, cfg *config.Config, projectRoot, markerPath, runID string, freshDeploy bool, w io.Writer) (*postinstall.Result, []distribution.StepResult, error) {
	resumeFrom, marker := resolveResumePhase(markerPath, cfg.Cluster.Name, freshDeploy)

	var setupSteps []distribution.StepResult
	var err error
	if resumeFrom == phasePrepare {
		setupSteps, err = runGuardedPrepare(ctx, p, cfg, markerPath, runID, freshDeploy, marker != nil, w)
		if err != nil {
			return nil, nil, err
		}
	} else {
		tui.Info("resuming interrupted deploy; skipping prepare to preserve cluster identity material",
			tui.LF("from_phase", string(resumeFrom)), tui.LF("interrupted_run_id", marker.RunID))
		tui.Info("to restart from scratch instead, re-run with --fresh (wipes cluster credentials)")
	}

	var installSteps []distribution.StepResult
	if resumeFrom != phaseConfigure {
		if err := markDeployPhaseFatal(markerPath, phaseInstall, runID, cfg.Cluster.Name); err != nil {
			return nil, nil, err
		}
		installOpts := install.NewOptions(cfg, projectRoot)
		installSteps, err = p.Install(ctx, cfg, &installOpts)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(w, render.InterruptSummary(slices.Concat(setupSteps, installSteps), "okdctl deploy", runID))
				tui.Info("cancelled during install — terraform state likely populated; run 'okdctl destroy' to clean up")
				return nil, nil, err
			}
			tui.Info("install failed — terraform state likely populated; run 'okdctl destroy' to clean up")
			return nil, nil, err
		}
	}

	if err := markDeployPhaseFatal(markerPath, phaseConfigure, runID, cfg.Cluster.Name); err != nil {
		return nil, nil, err
	}
	var result *postinstall.Result
	var configureSteps []distribution.StepResult
	if resumeFrom == phaseConfigure {
		result, configureSteps, err = p.ResumeConfigure(ctx, cfg)
	} else {
		result, configureSteps, err = p.Configure(ctx, cfg)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(w, render.InterruptSummary(slices.Concat(setupSteps, installSteps, configureSteps), "okdctl deploy", runID))
			tui.Info("cancelled during configure — terraform state likely populated; run 'okdctl destroy' to clean up")
			return nil, nil, err
		}
		tui.Info("configure failed — terraform state likely populated; run 'okdctl destroy' to clean up")
		return nil, nil, err
	}

	return result, slices.Concat(setupSteps, installSteps, configureSteps), nil
}

// Execute runs the full deployment — prepare, install, configure — under the
// project run lock, with resume routing, metrics, and the post-deploy summary.
func Execute(ctx context.Context, cfg *config.Config, opts Options, w io.Writer) error {
	projectRoot := opts.ProjectRoot

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
	p := NewProvisioner(opts.Credentials, projectRoot, provOpts...)
	defer p.ZeroizeEnv()

	if err := p.Validate(cfg); err != nil {
		return fmt.Errorf("provisioner validation failed: %w", err)
	}

	if opts.ShowStartMessage {
		tui.Info("starting deployment...", tui.LF("cluster", cfg.Cluster.Name+"."+cfg.Cluster.Domain))
	}

	startTime := time.Now()
	markerPath := filepath.Join(workDir, StateFileName)

	result, allSteps, err := runDeployPhases(ctx, p, cfg, projectRoot, markerPath, runID, opts.FreshDeploy, w)
	if err != nil {
		return err
	}

	clearDeployMarker(markerPath)

	duration := time.Since(startTime).Round(time.Second)

	fmt.Fprintln(w)
	tui.Info("deployment complete", tui.LF("duration", duration))
	fmt.Fprintln(w, render.PostDeploySummary(cfg, result, allSteps, runID))

	return nil
}
