// Package okd implements the OKD/OpenShift provisioner for Proxmox,
// delegating to phase-specific packages (setup, install, postinstall, destroy).
package okd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/destroy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Provisioner orchestrates the OKD distribution's phase packages (setup,
// install, postinstall, destroy, cleanup); construct via New, never the
// zero value.
type Provisioner struct {
	projectRoot string
	executor    *executor.Executor
	pendingEnv  []string
	logger      *slog.Logger
	recorder    distribution.MetricsRecorder
	reporter    logutil.ProgressReporter
	statusLine  logutil.StatusLineReporter
	streamOut   io.Writer
	streamErr   io.Writer
}

// ProvisionerOption configures a Provisioner; options compose via New.
type ProvisionerOption func(*Provisioner)

// WithProjectRoot sets the project-root working directory, defaulting to
// os.Getwd().
func WithProjectRoot(projectRoot string) ProvisionerOption {
	return func(p *Provisioner) {
		p.projectRoot = projectRoot
	}
}

// WithLogger attaches a structured logger; a nil logger normalizes to
// logutil.NopLogger.
func WithLogger(l *slog.Logger) ProvisionerOption {
	return func(p *Provisioner) {
		p.logger = logutil.OrNop(l)
	}
}

// WithMetricsRecorder attaches a MetricsRecorder for per-step and
// overall-run observations.
func WithMetricsRecorder(rec distribution.MetricsRecorder) ProvisionerOption {
	return func(p *Provisioner) { p.recorder = rec }
}

// WithProgressReporter sets the long-running-operation callback, defaulting
// to logutil.NopProgressReporter.
func WithProgressReporter(r logutil.ProgressReporter) ProvisionerOption {
	return func(p *Provisioner) { p.reporter = r }
}

// WithStatusLineReporter sets the status-line reporter the install monitor
// drives, defaulting to logutil.NopStatusLineReporter.
func WithStatusLineReporter(r logutil.StatusLineReporter) ProvisionerOption {
	return func(p *Provisioner) { p.statusLine = r }
}

// WithStreamWriters redirects subprocess stdout/stderr away from the
// executor's defaults. A nil writer keeps that stream on its default.
func WithStreamWriters(stdout, stderr io.Writer) ProvisionerOption {
	return func(p *Provisioner) {
		p.streamOut = stdout
		p.streamErr = stderr
	}
}

// WithEnv passes environment variables to the executor without touching the
// global process environment.
func WithEnv(env []string) ProvisionerOption {
	return func(p *Provisioner) {
		p.pendingEnv = append(p.pendingEnv, env...)
	}
}

// New constructs a Provisioner, applying opts in order and building the
// executor once afterward so WithEnv/WithLogger order doesn't matter.
func New(opts ...ProvisionerOption) *Provisioner {
	projectRoot, _ := os.Getwd()

	p := &Provisioner{
		projectRoot: projectRoot,
		logger:      logutil.NopLogger,
		reporter:    logutil.NopProgressReporter,
		statusLine:  logutil.NopStatusLineReporter,
	}

	for _, opt := range opts {
		opt(p)
	}

	execOpts := []executor.Option{executor.WithLogger(p.logger)}
	if len(p.pendingEnv) > 0 {
		execOpts = append(execOpts, executor.WithEnv(p.pendingEnv))
	}
	if p.streamOut != nil {
		execOpts = append(execOpts, executor.WithStdout(p.streamOut))
	}
	if p.streamErr != nil {
		execOpts = append(execOpts, executor.WithStderr(p.streamErr))
	}
	p.executor = executor.New(execOpts...)

	return p
}

// Validate checks the distribution type only; resource-minimum floors are
// enforced earlier by config.ValidateOKDConfig and deliberately not
// re-checked here.
func (p *Provisioner) Validate(cfg *config.Config) error {
	if cfg.Distribution.Type != config.DistributionOKD {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid distribution type: expected okd, got %s", cfg.Distribution.Type)}
	}
	return nil
}

// SetupOpts configures Provisioner.Setup. FreshDeploy allows wiping a work
// directory with live cluster state (otherwise Setup errors and forces
// destroy first); ResumeInProgress never unlocks that wipe — it only signals
// a setup-marker/state contradiction for Setup to refuse instead of trust.
type SetupOpts struct {
	FreshDeploy      bool
	ResumeInProgress bool
}

// Setup cleans up previous artifacts and runs the setup phase, refusing to
// wipe a work directory that looks like a live cluster unless
// opts.FreshDeploy is set.
func (p *Provisioner) Setup(ctx context.Context, cfg *config.Config, opts SetupOpts) ([]distribution.StepResult, error) {
	setupOpts := setup.NewOptions(cfg, p.projectRoot)

	if system.DirExists(setupOpts.WorkDir) {
		if err := p.guardLiveCluster(cfg, opts); err != nil {
			return nil, err
		}
		p.logger.Info("setup: cleaning up previous artifacts")
		cleanupOpts := cleanup.NewOptions(cfg, p.projectRoot, cleanup.WorkOnly)
		// guardLiveCluster already secured the --fresh credential-loss consent
		// this wipe needs.
		cleanupOpts.ForceCredentialWipe = opts.FreshDeploy
		if err := cleanup.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger)).Execute(ctx, &cleanupOpts); err != nil {
			return nil, &errtypes.ClusterError{Msg: "pre-deploy cleanup incomplete; stale sentinels may skip regeneration — remove the work directory manually or run 'okdctl cleanup'", Err: err}
		}
	}

	setupPhase := setup.New(
		phase.WithExecutor(p.executor),
		phase.WithLogger(p.logger),
		phase.WithRecorder(p.recorder),
	)
	setupPhase.BinDir = config.ResolveBinDir(cfg)
	return setupPhase.Execute(ctx, cfg, &setupOpts)
}

// GuardSetup reports, without side effects, whether Setup would refuse to
// wipe the work directory — call it before writing the deploy-state marker
// so a refusal can't plant a marker that bypasses the guard next run.
func (p *Provisioner) GuardSetup(cfg *config.Config, opts SetupOpts) error {
	setupOpts := setup.NewOptions(cfg, p.projectRoot)
	if !system.DirExists(setupOpts.WorkDir) {
		return nil
	}
	return p.guardLiveCluster(cfg, opts)
}

// guardLiveCluster errors when terraform state has resources — auth alone is
// mid-setup debris and stays wipeable; only FreshDeploy bypasses, accepting
// credential loss.
func (p *Provisioner) guardLiveCluster(cfg *config.Config, opts SetupOpts) error {
	if opts.FreshDeploy {
		return nil
	}
	tfEnv := cfg.TerraformEnvName()
	envDir := workspace.TerraformEnvDir(p.projectRoot, tfEnv)
	tf := terraform.New(envDir, terraform.WithLogger(p.logger))
	if !tf.HasState() {
		return nil
	}
	if opts.ResumeInProgress {
		return &errtypes.ConfigError{
			Msg: "deploy-state marker records a setup-phase interruption but terraform state has resources — " +
				"a setup run applies nothing, so the marker cannot be trusted; " +
				"run 'okdctl destroy' first, or pass --fresh to force-wipe (credentials will be lost)",
		}
	}
	return &errtypes.ConfigError{
		Msg: "terraform state has resources — the work directory belongs to a live cluster; " +
			"run 'okdctl destroy' first, or pass --fresh to force-wipe (credentials will be lost)",
	}
}

// Install runs the install phase: ignition delivery, bootstrap wait, and
// install-complete monitor. Must be called after Setup.
func (p *Provisioner) Install(ctx context.Context, cfg *config.Config, opts *install.Options) ([]distribution.StepResult, error) {
	installPhase := install.New(
		phase.WithExecutor(p.executor),
		phase.WithLogger(p.logger),
		phase.WithRecorder(p.recorder),
		phase.WithReporter(p.reporter),
		phase.WithStatusLine(p.statusLine),
	)
	return installPhase.Execute(ctx, cfg, opts)
}

// PostInstall runs the postinstall phase (kube-vip verification, production
// DNS cutover, bootstrap cleanup); keepRedHatCatalogs mirrors
// --keep-redhat-catalogs, skipping the RH catalog/Insights-alert step.
func (p *Provisioner) PostInstall(ctx context.Context, cfg *config.Config, keepRedHatCatalogs bool) (*postinstall.Result, []distribution.StepResult, error) {
	postPhase := postinstall.New(
		phase.WithExecutor(p.executor),
		phase.WithLogger(p.logger),
		phase.WithRecorder(p.recorder),
	)
	opts := postinstall.NewOptions(cfg, p.projectRoot)
	opts.KeepRedHatCatalogs = keepRedHatCatalogs
	return postPhase.Execute(ctx, cfg, &opts)
}

// ResumePostInstall re-runs postinstall for a deploy interrupted mid-phase
// without re-running Install — a terraform re-apply here would recreate the
// bootstrap VM against a live control plane. It only re-arms KUBECONFIG and
// fails fast if the kubeconfig is missing.
func (p *Provisioner) ResumePostInstall(ctx context.Context, cfg *config.Config, keepRedHatCatalogs bool) (*postinstall.Result, []distribution.StepResult, error) {
	installPhase := install.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	clusterDir := workspace.ClusterConfigDir(workspace.WorkDir(p.projectRoot))
	if err := installPhase.SetupKubeconfig(ctx, clusterDir); err != nil {
		return nil, nil, &errtypes.ClusterError{
			Msg: "cannot resume postinstall: cluster kubeconfig unavailable; " +
				"run 'okdctl destroy' then re-deploy, or re-run with --fresh to restart from scratch (credentials will be lost)",
			Err: err,
		}
	}
	return p.PostInstall(ctx, cfg, keepRedHatCatalogs)
}

// DeployStep is a step's identity for dry-run listings and the live
// checklist: ID, name, and owning phase — no executable body.
type DeployStep struct {
	ID    distribution.StepID
	Name  string
	Phase DeployPhase
}

// DeployPhase labels the deploy phase that owns a DeployStep.
type DeployPhase string

// Deploy phase labels for DeployStep.Phase; values match the deploy
// package's on-disk marker names so a resume can filter the plan.
const (
	PhaseSetup       DeployPhase = "setup"
	PhaseInstall     DeployPhase = "install"
	PhasePostInstall DeployPhase = "postinstall"
)

// DeploySteps returns the ordered ID+Name for every step across setup,
// install, and postinstall for cfg. It derives from the same StepDefs those
// phases feed into BuildSteps, so it can't drift from a phase's xSteps()
// method.
func (p *Provisioner) DeploySteps(cfg *config.Config) []DeployStep {
	setupPhase := setup.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	setupPhase.BinDir = config.ResolveBinDir(cfg)
	setupOpts := setup.NewOptions(cfg, p.projectRoot)

	installPhase := install.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	installOpts := install.NewOptions(cfg, p.projectRoot)

	postPhase := postinstall.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	postOpts := postinstall.NewOptions(cfg, p.projectRoot)

	var out []DeployStep
	appendPhase := func(phaseName DeployPhase, defs []distribution.StepDef) {
		for _, d := range defs {
			out = append(out, DeployStep{ID: d.ID, Name: d.Name, Phase: phaseName})
		}
	}
	appendPhase(PhaseSetup, setupPhase.StepDefs(cfg, &setupOpts))
	appendPhase(PhaseInstall, installPhase.StepDefs(cfg, &installOpts))
	appendPhase(PhasePostInstall, postPhase.StepDefs(cfg, &postOpts))
	return out
}

// UpdateIngress re-points haproxy at fresh backend nodes without re-running
// the full postinstall phase.
func (p *Provisioner) UpdateIngress(ctx context.Context, cfg *config.Config, opts postinstall.UpdateIngressOptions) (*postinstall.UpdateIngressResult, error) {
	postPhase := postinstall.New(
		phase.WithExecutor(p.executor),
		phase.WithLogger(p.logger),
	)
	opts.WorkDir = resolveIngressWorkDir(p.projectRoot, opts.WorkDir)
	return postPhase.UpdateIngress(ctx, cfg, opts)
}

// resolveIngressWorkDir defaults empty workDir to <projectRoot>/okd-install;
// WorkDir is the parent of cluster-config/, not the project root.
func resolveIngressWorkDir(projectRoot, workDir string) string {
	if workDir == "" {
		return workspace.WorkDir(projectRoot)
	}
	return workDir
}

// ZeroizeEnv blanks secret-keyed entries in pendingEnv and delegates to the
// executor's ZeroizeEnv, bounding plaintext credential lifetime on both
// copies. Call via defer after all phases complete.
func (p *Provisioner) ZeroizeEnv() {
	for i, kv := range p.pendingEnv {
		key, _, _ := strings.Cut(kv, "=")
		if logutil.KeyIsSecret(key) {
			p.pendingEnv[i] = ""
		}
	}
	clear(p.pendingEnv)
	p.pendingEnv = nil
	if p.executor == nil {
		return
	}
	p.executor.ZeroizeEnv()
}

// Destroy tears down the cluster and its infrastructure; build opts via
// destroy.NewOptions(cfg, projectRoot) — there is no separate CLI-facing
// options type.
func (p *Provisioner) Destroy(ctx context.Context, cfg *config.Config, opts *destroy.Options) ([]distribution.StepResult, error) {
	destroyPhase := destroy.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	return destroyPhase.Execute(ctx, cfg, opts)
}

// Cleanup removes local cluster artifacts without touching infrastructure;
// build opts via cleanup.NewOptions(cfg, projectRoot, kind), the same
// pass-through contract as Destroy. Setup's internal pre-deploy cleanup does
// not route through here — it wires its own WorkOnly run.
func (p *Provisioner) Cleanup(ctx context.Context, opts *cleanup.Options) error {
	cleanupPhase := cleanup.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	return cleanupPhase.Execute(ctx, opts)
}
