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
func runGuardedSetup(ctx context.Context, p provisioner, cfg *config.Config, markerPath, runID string, freshDeploy, resumeInProgress bool, started time.Time, pr presenter) ([]distribution.StepResult, error) {
	setupOpts := okd.SetupOpts{FreshDeploy: freshDeploy, ResumeInProgress: resumeInProgress && !freshDeploy}
	if err := p.GuardSetup(cfg, setupOpts); err != nil {
		return nil, err
	}

	if err := markDeployPhaseFatal(markerPath, phaseSetup, runID, cfg.Cluster.Name); err != nil {
		return nil, err
	}
	setupSteps, err := p.Setup(ctx, cfg, setupOpts)
	if err != nil {
		return setupSteps, pr.fail(err, phaseSetup, setupSteps, runID, started,
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

// presenter decides who renders a phase failure. A plain run prints the
// failure box to w and marks the error presented; a TUI-driven run (Options.
// Reporter set) leaves both to the caller, which renders in-frame and reports
// once the screen is released.
type presenter struct {
	w       io.Writer
	enabled bool
}

// fail reports err for phase, logs cancelHint on a cancelled run, and returns
// the error the caller should propagate.
func (p presenter) fail(err error, phase deployPhase, steps []distribution.StepResult, runID string, started time.Time, cancelHint string) error {
	if p.enabled {
		reportDeployFailure(p.w, err, phase, steps, runID, started)
	}
	if errors.Is(err, context.Canceled) {
		logutil.Info(cancelHint)
	}
	if !p.enabled {
		return err
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
	// Reporter receives per-step progress from the orchestrator while a TUI
	// owns the screen, in place of the stderr checklist. Execute writes no
	// summary or failure box to w when it is set: the TUI renders both
	// in-frame and the caller reports once the screen is released.
	Reporter distribution.MetricsRecorder
}

// Outcome is what a finished Execute produced: the postinstall result, every
// step it executed, the run correlation id, and the wall-clock total — what
// the post-deploy summary box is rendered from.
type Outcome struct {
	Result   *postinstall.Result
	Steps    []distribution.StepResult
	RunID    string
	Duration time.Duration
}

// checklistRecorder builds a TTY step-checklist seeded only with steps this
// resumed run executes (so N/total matches), or nil when progress rendering
// is off; logSink still gets the per-step trail the checklist replaces on
// the TTY. The return type is the concrete *tui.StepProgress, not the
// distribution.MetricsRecorder interface, so the call site can read Prefix() directly.
func checklistRecorder(cfg *config.Config, projectRoot string, resumeFrom deployPhase, logSink io.Writer) *tui.StepProgress {
	if !logutil.ProgressBarsEnabled() {
		return nil
	}
	plan := plannedSteps(cfg, projectRoot, resumeFrom)
	if len(plan) == 0 {
		return nil
	}
	return tui.NewStepProgress(plan, logSink)
}

// PlannedSteps returns the checklist plan for the next deploy run against
// projectRoot: every registered step whose phase that run executes, given the
// on-disk resume marker. A TUI checklist seeds its rows from this so its
// N/total matches whatever Execute goes on to run.
func PlannedSteps(cfg *config.Config, projectRoot string, freshDeploy bool) []tui.StepMeta {
	markerPath := filepath.Join(workspace.WorkDir(projectRoot), StateFileName)
	resumeFrom, _ := resolveResumePhase(markerPath, cfg.Cluster.Name, freshDeploy)
	return plannedSteps(cfg, projectRoot, resumeFrom)
}

// plannedSteps derives the step plan from live phase StepDefs, dropping the
// phases a resume from resumeFrom skips, so it cannot drift from what the
// orchestrator runs.
func plannedSteps(cfg *config.Config, projectRoot string, resumeFrom deployPhase) []tui.StepMeta {
	all := okd.New(okd.WithProjectRoot(projectRoot), okd.WithLogger(logutil.SimpleLogger())).DeploySteps(cfg)
	var plan []tui.StepMeta
	for _, s := range all {
		if !phaseRuns(resumeFrom, s.Phase) {
			continue
		}
		plan = append(plan, tui.StepMeta{ID: s.ID, Name: s.Name, Phase: string(s.Phase)})
	}
	return plan
}

// checklistPrefix returns rec's current-step prefix, or "" when rec is nil
// (progress rendering off or nothing to show between steps).
func checklistPrefix(rec *tui.StepProgress) string {
	if rec == nil {
		return ""
	}
	return rec.Prefix()
}

// phaseRuns reports whether stepPhase executes given resumeFrom: a resume
// runs its entry phase and every later phase.
func phaseRuns(resumeFrom deployPhase, stepPhase okd.DeployPhase) bool {
	order := map[okd.DeployPhase]int{okd.PhaseSetup: 0, okd.PhaseInstall: 1, okd.PhasePostInstall: 2}
	return order[stepPhase] >= order[okd.DeployPhase(resumeFrom)]
}

// runDeployPhases runs setup/install/postinstall from the marker's resume
// phase, returning the postinstall result and every executed step.
func runDeployPhases(ctx context.Context, p provisioner, cfg *config.Config, projectRoot, markerPath, runID string, resumeFrom deployPhase, marker *deployState, freshDeploy, keepRedHatCatalogs bool, started time.Time, pr presenter) (*postinstall.Result, []distribution.StepResult, error) {
	var setupSteps []distribution.StepResult
	var err error
	if resumeFrom == phaseSetup {
		setupSteps, err = runGuardedSetup(ctx, p, cfg, markerPath, runID, freshDeploy, marker != nil, started, pr)
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
			return nil, nil, pr.fail(err, phaseInstall, slices.Concat(setupSteps, installSteps), runID, started,
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
		return nil, nil, pr.fail(err, phasePostInstall, slices.Concat(setupSteps, installSteps, postinstallSteps), runID, started,
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
// Options.Reporter suppresses every box it would write, leaving the returned
// outcome for the caller to present.
func Execute(ctx context.Context, cfg *config.Config, opts *Options, w io.Writer) (*Outcome, error) {
	projectRoot := opts.ProjectRoot

	lock, err := runlock.Acquire(projectRoot, "deploy")
	if err != nil {
		return nil, err
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

	// A TUI-driven run owns the screen, so the stderr checklist is never built:
	// its rewriting line would paint under the AltScreen.
	var rec *tui.StepProgress
	if opts.Reporter == nil {
		rec = checklistRecorder(cfg, projectRoot, resumeFrom, opts.LogSink)
	}
	provOpts := []okd.ProvisionerOption{
		okd.WithProgressReporter(func(desc string) func() {
			return tui.StartSpinnerWithPrefix(ctx, checklistPrefix(rec), desc)
		}),
		okd.WithStatusLineReporter(func(desc string) (func(string), func()) {
			return tui.StartStatusLineWithPrefix(ctx, checklistPrefix(rec), desc)
		}),
	}
	switch {
	case opts.Reporter != nil:
		provOpts = append(provOpts, okd.WithMetricsRecorder(opts.Reporter))
	case rec != nil:
		provOpts = append(provOpts, okd.WithMetricsRecorder(rec))
	}
	if so, se := streamWriters(opts.LogSink, opts.Verbose); so != nil {
		provOpts = append(provOpts, okd.WithStreamWriters(so, se))
	}
	p := NewProvisioner(opts.Credentials, projectRoot, provOpts...)
	defer p.ZeroizeEnv()

	if err := p.Validate(cfg); err != nil {
		return nil, fmt.Errorf("validate deploy config: %w", err)
	}

	if opts.ShowStartMessage {
		logutil.Info("starting deployment...",
			logutil.LF("cluster", cfg.Cluster.Name), logutil.LF("domain", cfg.Cluster.Domain))
	}

	startTime := time.Now()

	pr := presenter{w: w, enabled: opts.Reporter == nil}
	result, allSteps, err := runDeployPhases(ctx, p, cfg, projectRoot, markerPath, runID, resumeFrom, marker, opts.FreshDeploy, opts.KeepRedHatCatalogs, startTime, pr)
	if err != nil {
		return nil, err
	}

	clearDeployMarker(markerPath, runID, cfg.Cluster.Name)

	duration := time.Since(startTime).Round(time.Second)
	// Logged before the presenter gate so a TUI-driven run records its
	// completion in okdctl.log too, where its only durable record lives.
	logutil.Info("deployment complete", logutil.LF("duration", duration))

	outcome := &Outcome{Result: result, Steps: allSteps, RunID: runID, Duration: duration}
	if !pr.enabled {
		return outcome, nil
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, render.PostDeploySummary(cfg, result, allSteps, runID))

	return outcome, nil
}
