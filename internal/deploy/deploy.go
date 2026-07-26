package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
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

// streamWriters builds the stdout/stderr writers for the phase executor from
// the persistent log sink. Default (not verbose): the openshift-install
// firehose routes to the log file only, leaving the TTY for the curated
// status line and milestone lines. Verbose: the stream is tee'd to both the
// TTY (raw) and the log file. In every mode the stream is scanned for
// milestones (bootstrap/install complete, degraded operators) promoted to the
// TTY. Returns (nil, nil) when no sink is active, so the caller leaves the
// executor on its os.Stdout/os.Stderr defaults.
func streamWriters(sink io.Writer, verbose bool) (stdout, stderr io.Writer) {
	if sink == nil {
		return nil, nil
	}
	baseOut, baseErr := sink, sink
	if verbose {
		baseOut = io.MultiWriter(os.Stdout, sink)
		baseErr = io.MultiWriter(os.Stderr, sink)
	}
	notify := milestoneNotifier()
	return install.NewMilestoneWriter(baseOut, notify), install.NewMilestoneWriter(baseErr, notify)
}

// milestoneNotifier returns a notify func that promotes each openshift-install
// milestone to the TTY log at most once per distinct event — openshift-install
// re-prints a degraded operator on every poll, so a seen-set keeps the status
// feed from spamming. The func is called from both the stdout and stderr copy
// goroutines, so its map is mutex-guarded.
func milestoneNotifier() func(install.Milestone) {
	var mu sync.Mutex
	seen := map[string]bool{}
	return func(m install.Milestone) {
		var key string
		switch m.Kind {
		case install.MilestoneBootstrapComplete:
			key = "bootstrap"
		case install.MilestoneInstallComplete:
			key = "install"
		case install.MilestoneOperatorDegraded:
			key = "degraded:" + m.Operator
		default:
			return
		}
		mu.Lock()
		if seen[key] {
			mu.Unlock()
			return
		}
		seen[key] = true
		mu.Unlock()

		switch m.Kind {
		case install.MilestoneBootstrapComplete:
			tui.Info("bootstrap complete — control plane has taken over")
		case install.MilestoneInstallComplete:
			tui.Info("install complete — cluster is initialized")
		case install.MilestoneOperatorDegraded:
			tui.Warn("cluster operator degraded during install", tui.LF("operator", m.Operator))
		}
	}
}

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
	GuardSetup(cfg *config.Config, opts okd.SetupOpts) error
	Setup(ctx context.Context, cfg *config.Config, opts okd.SetupOpts) ([]distribution.StepResult, error)
	Install(ctx context.Context, cfg *config.Config, opts *install.Options) ([]distribution.StepResult, error)
	PostInstall(ctx context.Context, cfg *config.Config, keepRedHatCatalogs bool) (*postinstall.Result, []distribution.StepResult, error)
	ResumePostInstall(ctx context.Context, cfg *config.Config, keepRedHatCatalogs bool) (*postinstall.Result, []distribution.StepResult, error)
}

// runGuardedSetup runs the setup phase behind the live-cluster guard.
// resumeInProgress carries the caller's marker decision: only a setup-phase
// marker for this cluster reaches here (resolveResumePhase routes install and
// postinstall markers past Setup entirely), so the guard bypass cannot wipe
// material live VMs booted with. The guard probe runs before any marker
// write — a refusal must not plant a marker that would bypass the guard on
// the next invocation.
func runGuardedSetup(ctx context.Context, p provisioner, cfg *config.Config, markerPath, runID string, freshDeploy, resumeInProgress bool, started time.Time, w io.Writer) ([]distribution.StepResult, error) {
	setupOpts := okd.SetupOpts{FreshDeploy: freshDeploy, ResumeInProgress: resumeInProgress && !freshDeploy}
	if err := p.GuardSetup(cfg, setupOpts); err != nil {
		return nil, err
	}

	if err := markDeployPhaseFatal(markerPath, phaseSetup, runID, cfg.Cluster.Name); err != nil {
		return nil, err
	}
	setupSteps, err := p.Setup(ctx, cfg, setupOpts)
	if err != nil {
		return setupSteps, reportDeployPhaseError(w, err, phaseSetup, setupSteps, runID, started,
			"cancelled during setup — terraform state is empty; run 'okdctl cleanup' to remove local files")
	}
	return setupSteps, nil
}

// reportDeployFailure prints the end-of-run box for a phase error. Cancelled
// runs keep the interrupt box; every other failure gets a failure summary
// whose resume line names the phase the on-disk marker recorded just before
// the phase ran, so re-running deploy truthfully resumes there. Setup
// applies nothing to Proxmox, so its teardown alternative is cleanup —
// destroy would be a misleading no-op.
func reportDeployFailure(w io.Writer, err error, phase deployPhase, steps []distribution.StepResult, runID string, started time.Time) {
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(w, render.InterruptSummary(steps, "okdctl deploy", runID))
		return
	}
	teardownCmd, teardownNote := "okdctl destroy", "remove provisioned resources"
	if phase == phaseSetup {
		teardownCmd, teardownNote = "okdctl cleanup", "remove local files (terraform state is empty)"
	}
	fmt.Fprintln(w, render.FailureSummary(&render.FailureInfo{
		Steps:        steps,
		Phase:        string(phase),
		RunID:        runID,
		Elapsed:      time.Since(started),
		TeardownCmd:  teardownCmd,
		TeardownNote: teardownNote,
	}))
}

// reportDeployPhaseError prints the end-of-run box via reportDeployFailure
// and, for a cancelled run, logs cancelHint — the phase-specific guidance on
// terraform-state disposition. Returns err unchanged so callers can fold it
// into their own return shape.
func reportDeployPhaseError(w io.Writer, err error, phase deployPhase, steps []distribution.StepResult, runID string, started time.Time, cancelHint string) error {
	reportDeployFailure(w, err, phase, steps, runID, started)
	if errors.Is(err, context.Canceled) {
		tui.Info(cancelHint)
	}
	// The FailureSummary/InterruptSummary box above already presents this
	// failure; mark it so the top-level handler does not stack a second box.
	return render.Presented(err)
}

// Options configures Execute. ProjectRoot must be a resolved project root
// (see the cli package's project-marker validation).
type Options struct {
	ShowStartMessage   bool
	Credentials        *credentials.ProxmoxCredentials
	FreshDeploy        bool
	KeepRedHatCatalogs bool
	ProjectRoot        string
	// LogSink is the persistent okdctl.log writer. When set, the
	// openshift-install firehose routes there instead of the TTY; nil leaves
	// streamed output on the default os.Stdout/os.Stderr.
	LogSink io.Writer
	// Verbose mirrors --verbose: it keeps the streamed subprocess output on
	// the TTY (tee'd to LogSink) instead of routing it to the log file only.
	Verbose bool
}

// checklistRecorder builds the live step-checklist recorder for a TTY deploy,
// seeded with only the steps the resumed run will actually execute so the
// counter (N/total) reflects this run rather than a full deploy. Returns nil
// when progress rendering is off (non-TTY or JSON), leaving the orchestrator's
// plain step log lines in place. logSink receives the per-step trail so
// okdctl.log keeps a record the checklist otherwise replaces on the TTY.
func checklistRecorder(cfg *config.Config, projectRoot string, resumeFrom deployPhase, logSink io.Writer) distribution.MetricsRecorder {
	if !tui.ProgressBarsEnabled() {
		return nil
	}
	all := okd.New(okd.WithProjectRoot(projectRoot), okd.WithLogger(tui.SimpleLogger())).DeploySteps(cfg)
	var plan []tui.StepMeta
	for _, s := range all {
		if !phaseRuns(resumeFrom, s.Phase) {
			continue
		}
		plan = append(plan, tui.StepMeta{ID: s.ID, Name: s.Name, Phase: s.Phase})
	}
	if len(plan) == 0 {
		return nil
	}
	return tui.NewStepProgress(plan, logSink)
}

// phaseRuns reports whether a step in stepPhase executes given resumeFrom as
// the entry point: a resume runs its entry phase and every later phase.
func phaseRuns(resumeFrom deployPhase, stepPhase string) bool {
	order := map[string]int{okd.PhaseSetup: 0, okd.PhaseInstall: 1, okd.PhasePostInstall: 2}
	return order[stepPhase] >= order[string(resumeFrom)]
}

// runDeployPhases executes setup, install, and postinstall, starting from
// the phase the deploy-state marker says is safe to resume from. Returns the
// postinstall result and every executed step across phases.
func runDeployPhases(ctx context.Context, p provisioner, cfg *config.Config, projectRoot, markerPath, runID string, resumeFrom deployPhase, marker *deployState, freshDeploy, keepRedHatCatalogs bool, started time.Time, w io.Writer) (*postinstall.Result, []distribution.StepResult, error) {
	var setupSteps []distribution.StepResult
	var err error
	if resumeFrom == phaseSetup {
		setupSteps, err = runGuardedSetup(ctx, p, cfg, markerPath, runID, freshDeploy, marker != nil, started, w)
		if err != nil {
			return nil, nil, err
		}
	} else {
		tui.Info("resuming interrupted deploy; skipping setup to preserve cluster identity material",
			tui.LF("from_phase", string(resumeFrom)), tui.LF("interrupted_run_id", marker.RunID))
		tui.Info("to restart from scratch instead, re-run with --fresh (wipes cluster credentials)")
	}

	var installSteps []distribution.StepResult
	if resumeFrom != phasePostInstall {
		if err := markDeployPhaseFatal(markerPath, phaseInstall, runID, cfg.Cluster.Name); err != nil {
			return nil, nil, err
		}
		installOpts := install.NewOptions(cfg, projectRoot)
		installSteps, err = p.Install(ctx, cfg, &installOpts)
		if err != nil {
			return nil, nil, reportDeployPhaseError(w, err, phaseInstall, slices.Concat(setupSteps, installSteps), runID, started,
				"cancelled during install — terraform state likely populated; run 'okdctl destroy' to clean up")
		}
	}

	if err := markDeployPhaseFatal(markerPath, phasePostInstall, runID, cfg.Cluster.Name); err != nil {
		return nil, nil, err
	}
	var result *postinstall.Result
	var postinstallSteps []distribution.StepResult
	if resumeFrom == phasePostInstall {
		result, postinstallSteps, err = p.ResumePostInstall(ctx, cfg, keepRedHatCatalogs)
	} else {
		result, postinstallSteps, err = p.PostInstall(ctx, cfg, keepRedHatCatalogs)
	}
	if err != nil {
		return nil, nil, reportDeployPhaseError(w, err, phasePostInstall, slices.Concat(setupSteps, installSteps, postinstallSteps), runID, started,
			"cancelled during postinstall — terraform state likely populated; run 'okdctl destroy' to clean up")
	}

	return result, slices.Concat(setupSteps, installSteps, postinstallSteps), nil
}

// Execute runs the full deployment — setup, install, postinstall — under the
// project run lock, with resume routing and the post-deploy summary.
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
	workDir := filepath.Join(projectRoot, system.WorkDirName)
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			tui.Warn("workdir chown back to user incomplete", tui.LF("err", chownErr))
		}
	}()

	runID := tui.RunID()

	markerPath := filepath.Join(workDir, StateFileName)
	resumeFrom, marker := resolveResumePhase(markerPath, cfg.Cluster.Name, opts.FreshDeploy)

	provOpts := []okd.ProvisionerOption{
		okd.WithProgressReporter(func(desc string) func() { return tui.StartSpinner(ctx, desc) }),
		okd.WithStatusLineReporter(func(desc string) (func(string), func()) { return tui.StartStatusLine(ctx, desc) }),
	}
	if rec := checklistRecorder(cfg, projectRoot, resumeFrom, opts.LogSink); rec != nil {
		provOpts = append(provOpts, okd.WithMetricsRecorder(rec))
	}
	if so, se := streamWriters(opts.LogSink, opts.Verbose); so != nil {
		provOpts = append(provOpts, okd.WithStreamWriters(so, se))
	}
	p := NewProvisioner(opts.Credentials, projectRoot, provOpts...)
	defer p.ZeroizeEnv()

	if err := p.Validate(cfg); err != nil {
		return fmt.Errorf("provisioner validation failed: %w", err)
	}

	if opts.ShowStartMessage {
		tui.Info("starting deployment...", tui.LF("cluster", cfg.Cluster.Name+"."+cfg.Cluster.Domain))
	}

	startTime := time.Now()

	result, allSteps, err := runDeployPhases(ctx, p, cfg, projectRoot, markerPath, runID, resumeFrom, marker, opts.FreshDeploy, opts.KeepRedHatCatalogs, startTime, w)
	if err != nil {
		return err
	}

	clearDeployMarker(markerPath, runID, cfg.Cluster.Name)

	duration := time.Since(startTime).Round(time.Second)

	fmt.Fprintln(w)
	tui.Info("deployment complete", tui.LF("duration", duration))
	fmt.Fprintln(w, render.PostDeploySummary(cfg, result, allSteps, runID))

	return nil
}
