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
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// StateFileName is the deploy-state marker file under <projectRoot>/okd-install.
const StateFileName = ".okdctl-deploy-state.json"

// streamWriters routes the stream to sink only by default, tees to the TTY
// when verbose, and returns (nil, nil) when sink is nil.
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

// milestoneNotifier dedupes each milestone to one TTY log line, guarded by a
// mutex shared across the stdout/stderr copy goroutines.
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
			logutil.Info("bootstrap complete — control plane has taken over")
		case install.MilestoneInstallComplete:
			logutil.Info("install complete — cluster is initialized")
		case install.MilestoneOperatorDegraded:
			logutil.Warn("cluster operator degraded during install", logutil.LF("operator", m.Operator))
		}
	}
}

// NewProvisioner builds an okd.Provisioner wired for CLI use. Callers must
// defer p.ZeroizeEnv().
func NewProvisioner(creds *credentials.ProxmoxCredentials, projectRoot string, extra ...okd.ProvisionerOption) *okd.Provisioner {
	opts := []okd.ProvisionerOption{
		okd.WithProjectRoot(projectRoot),
		okd.WithLogger(logutil.SimpleLogger()),
	}

	if creds != nil && creds.IsValid() {
		opts = append(opts, okd.WithEnv(creds.Env()))
	}

	opts = append(opts, extra...)
	return okd.New(opts...)
}

// provisioner is the interface the deploy flow drives; tests substitute a fake.
type provisioner interface {
	GuardSetup(cfg *config.Config, opts okd.SetupOpts) error
	Setup(ctx context.Context, cfg *config.Config, opts okd.SetupOpts) ([]distribution.StepResult, error)
	Install(ctx context.Context, cfg *config.Config, opts *install.Options) ([]distribution.StepResult, error)
	PostInstall(ctx context.Context, cfg *config.Config, keepRedHatCatalogs bool) (*postinstall.Result, []distribution.StepResult, error)
	ResumePostInstall(ctx context.Context, cfg *config.Config, keepRedHatCatalogs bool) (*postinstall.Result, []distribution.StepResult, error)
}

// runGuardedSetup runs the guard before writing any marker, so a refusal
// can't plant a marker that bypasses the guard next run.
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

// reportDeployFailure prints the failure/interrupt box; a setup-phase
// failure points at cleanup instead of destroy since terraform state is empty.
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

// reportDeployPhaseError reports the failure, logs cancelHint on a
// cancelled run, and returns err unchanged.
func reportDeployPhaseError(w io.Writer, err error, phase deployPhase, steps []distribution.StepResult, runID string, started time.Time, cancelHint string) error {
	reportDeployFailure(w, err, phase, steps, runID, started)
	if errors.Is(err, context.Canceled) {
		logutil.Info(cancelHint)
	}
	// Already presented above; mark it so the top-level handler doesn't stack a second box.
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
	// LogSink is the persistent okdctl.log writer; nil leaves streamed
	// output on the default os.Stdout/os.Stderr.
	LogSink io.Writer
	// Verbose keeps streamed subprocess output on the TTY (tee'd to LogSink)
	// instead of routing it to the log file only.
	Verbose bool
}

// checklistRecorder builds a TTY step-checklist seeded only with steps this
// resumed run executes (so N/total matches), or nil when progress rendering
// is off; logSink still gets the per-step trail the checklist replaces on the TTY.
func checklistRecorder(cfg *config.Config, projectRoot string, resumeFrom deployPhase, logSink io.Writer) distribution.MetricsRecorder {
	if !logutil.ProgressBarsEnabled() {
		return nil
	}
	all := okd.New(okd.WithProjectRoot(projectRoot), okd.WithLogger(logutil.SimpleLogger())).DeploySteps(cfg)
	var plan []tui.StepMeta
	for _, s := range all {
		if !phaseRuns(resumeFrom, s.Phase) {
			continue
		}
		plan = append(plan, tui.StepMeta{ID: s.ID, Name: s.Name, Phase: string(s.Phase)})
	}
	if len(plan) == 0 {
		return nil
	}
	return tui.NewStepProgress(plan, logSink)
}

// phaseRuns reports whether stepPhase executes given resumeFrom: a resume
// runs its entry phase and every later phase.
func phaseRuns(resumeFrom deployPhase, stepPhase okd.DeployPhase) bool {
	order := map[okd.DeployPhase]int{okd.PhaseSetup: 0, okd.PhaseInstall: 1, okd.PhasePostInstall: 2}
	return order[stepPhase] >= order[okd.DeployPhase(resumeFrom)]
}

// runDeployPhases runs setup/install/postinstall from the marker's resume
// phase, returning the postinstall result and every executed step.
func runDeployPhases(ctx context.Context, p provisioner, cfg *config.Config, projectRoot, markerPath, runID string, resumeFrom deployPhase, marker *deployState, freshDeploy, keepRedHatCatalogs bool, started time.Time, w io.Writer) (*postinstall.Result, []distribution.StepResult, error) {
	var setupSteps []distribution.StepResult
	var err error
	if resumeFrom == phaseSetup {
		setupSteps, err = runGuardedSetup(ctx, p, cfg, markerPath, runID, freshDeploy, marker != nil, started, w)
		if err != nil {
			return nil, nil, err
		}
	} else {
		logutil.Info("resuming interrupted deploy; skipping setup to preserve cluster identity material",
			logutil.LF("from_phase", string(resumeFrom)), logutil.LF("interrupted_run_id", marker.RunID))
		logutil.Info("to restart from scratch instead, re-run with --fresh (wipes cluster credentials)")
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

// announceEmbeddedDrift warns when the write-once terraform workspace lags
// this binary's embedded sources; it only warns because refreshing
// operator-edited HCL without consent would break the write-once contract.
func announceEmbeddedDrift(root string) {
	drift, err := DetectEmbeddedDrift(root)
	if err != nil {
		logutil.Warn("could not compare terraform workspace against embedded sources", logutil.LF("err", err))
		return
	}
	for _, f := range drift.Stale {
		logutil.Warn("terraform file was written by an older okdctl and differs from this binary's embedded copy; the workspace copy is kept (write-once) — back it up and delete it, then re-run deploy to refresh it",
			logutil.LF("path", f))
	}
	for _, f := range drift.Unverified {
		logutil.Warn("terraform file differs from this binary's embedded copy; if you did not edit it, back it up and delete it, then re-run deploy to refresh it",
			logutil.LF("path", f))
	}
}

// Execute runs the full deploy pipeline under the project run lock, resuming
// from the on-disk marker's phase and writing the post-deploy summary to w.
func Execute(ctx context.Context, cfg *config.Config, opts Options, w io.Writer) error {
	projectRoot := opts.ProjectRoot

	lock, err := runlock.Acquire(projectRoot, "deploy")
	if err != nil {
		return err
	}
	defer lock.Release()

	// Under the sudo re-exec model, per-run artifacts under okd-install are
	// root-owned; restore ownership to the invoking user at exit so they can
	// inspect/rm -rf without sudo (no-op outside sudo).
	workDir := workspace.WorkDir(projectRoot)
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			logutil.Warn("workdir chown back to user incomplete", logutil.LF("err", chownErr))
		}
	}()

	runID := logutil.RunID()

	announceEmbeddedDrift(projectRoot)

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
		return fmt.Errorf("validate deploy config: %w", err)
	}

	if opts.ShowStartMessage {
		logutil.Info("starting deployment...",
			logutil.LF("cluster", cfg.Cluster.Name), logutil.LF("domain", cfg.Cluster.Domain))
	}

	startTime := time.Now()

	result, allSteps, err := runDeployPhases(ctx, p, cfg, projectRoot, markerPath, runID, resumeFrom, marker, opts.FreshDeploy, opts.KeepRedHatCatalogs, startTime, w)
	if err != nil {
		return err
	}

	clearDeployMarker(markerPath, runID, cfg.Cluster.Name)

	duration := time.Since(startTime).Round(time.Second)

	fmt.Fprintln(w)
	logutil.Info("deployment complete", logutil.LF("duration", duration))
	fmt.Fprintln(w, render.PostDeploySummary(cfg, result, allSteps, runID))

	return nil
}
