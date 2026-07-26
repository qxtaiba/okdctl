// Package okd implements the OKD/OpenShift provisioner for Proxmox.
// The Provisioner delegates to phase-specific packages (setup, install, postinstall, destroy).
package okd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

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
)

// Provisioner orchestrates the OKD distribution's phase packages (setup,
// install, postinstall, destroy, cleanup). Construct via New with
// functional options; the zero value is not usable.
//
// Options convention: Setup takes the facade-owned SetupOpts because it
// encodes wipe-guard consent rather than phase tuning; Install, Destroy,
// UpdateIngress, and Cleanup pass their phase package's Options through
// unchanged so there is no CLI-facing mirror type to keep in sync.
// PostInstall's bare keepRedHatCatalogs bool is grandfathered — fold it
// into an options struct when a second knob lands rather than adding a
// second positional flag.
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

// ProvisionerOption configures a Provisioner. Options compose — pass multiple
// to New. Nil-safe where documented (WithLogger accepts nil).
type ProvisionerOption func(*Provisioner)

// WithProjectRoot sets the working directory rooted at the okdctl project
// checkout. Defaults to os.Getwd() when omitted.
func WithProjectRoot(projectRoot string) ProvisionerOption {
	return func(p *Provisioner) {
		p.projectRoot = projectRoot
	}
}

// WithLogger attaches a structured logger. A nil logger is tolerated and
// normalized to logutil.NopLogger inside New.
func WithLogger(l *slog.Logger) ProvisionerOption {
	return func(p *Provisioner) {
		p.logger = l
	}
}

// WithMetricsRecorder attaches a MetricsRecorder that receives per-step and
// overall-run observations during the provisioner's Execute phases.
func WithMetricsRecorder(rec distribution.MetricsRecorder) ProvisionerOption {
	return func(p *Provisioner) { p.recorder = rec }
}

// WithProgressReporter sets the callback used by phases to signal long-running
// operations. Defaults to logutil.NopProgressReporter when omitted.
func WithProgressReporter(r logutil.ProgressReporter) ProvisionerOption {
	return func(p *Provisioner) { p.reporter = r }
}

// WithStatusLineReporter sets the updatable status-line reporter the install
// monitor drives during the cluster-operator wait. Defaults to
// logutil.NopStatusLineReporter when omitted.
func WithStatusLineReporter(r logutil.StatusLineReporter) ProvisionerOption {
	return func(p *Provisioner) { p.statusLine = r }
}

// WithStreamWriters redirects streamed subprocess stdout/stderr away from the
// executor's os.Stdout/os.Stderr defaults — deploy routes the openshift-install
// firehose to the persistent log file so the TTY carries only the curated
// status line. A nil writer leaves that stream on its default.
func WithStreamWriters(stdout, stderr io.Writer) ProvisionerOption {
	return func(p *Provisioner) {
		p.streamOut = stdout
		p.streamErr = stderr
	}
}

// WithEnv passes environment variables to the executor for all subprocess calls,
// avoiding modification of the global process environment.
func WithEnv(env []string) ProvisionerOption {
	return func(p *Provisioner) {
		p.pendingEnv = append(p.pendingEnv, env...)
	}
}

// New constructs a Provisioner with options applied in order. Normalizes a
// nil logger to NopLogger and builds the executor once after all options
// are applied so WithEnv/WithLogger ordering does not matter.
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

// Validate checks the distribution type and delegates resource-minimum
// validation to config-side ValidateOKDConfig so a single source of truth
// is enforced (config-load time and provisioner entry both hit it).
func (p *Provisioner) Validate(cfg *config.Config) error {
	if cfg.Distribution.Type != config.DistributionOKD {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("invalid distribution type: expected okd, got %s", cfg.Distribution.Type)}
	}
	return nil
}

// SetupOpts configures a Provisioner.Setup run. FreshDeploy permits
// wiping a work directory that contains live cluster state (terraform
// resources or cluster-config/auth); without it Setup returns an error
// so the operator is forced to destroy the cluster first.
//
// ResumeInProgress signals that the CLI's deploy-state marker records a
// setup-phase interruption for this cluster. A setup run applies
// nothing to Proxmox, so populated terraform state alongside such a
// marker is a contradiction (a failed install-marker write, or a --fresh
// run interrupted over an older cluster); the guard then refuses with a
// contradiction diagnostic instead of unlocking the wipe. Markers
// recording an install or postinstall interruption must never reach
// Setup: the CLI routes those resumes past the wipe entirely
// (deploy.resolveResumePhase) because the work directory then holds
// identity material live VMs depend on.
type SetupOpts struct {
	FreshDeploy      bool
	ResumeInProgress bool
}

// Setup cleans up previous artifacts and runs the setup phase. It refuses
// to wipe a work directory that appears to belong to a live cluster unless
// opts.FreshDeploy is true; pass --fresh at the CLI to opt in.
func (p *Provisioner) Setup(ctx context.Context, cfg *config.Config, opts SetupOpts) ([]distribution.StepResult, error) {
	setupOpts := setup.NewOptions(cfg, p.projectRoot)

	if system.DirExists(setupOpts.WorkDir) {
		if err := p.guardLiveCluster(cfg, opts); err != nil {
			return nil, err
		}
		p.logger.Info("setup: cleaning up previous artifacts")
		cleanupOpts := cleanup.NewOptions(cfg, p.projectRoot, cleanup.WorkOnly)
		// guardLiveCluster already obtained the credential-loss consent that
		// --fresh implies; without it the wipe must still honor live state.
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

// GuardSetup reports whether Setup would refuse to wipe the work
// directory, without side effects. The CLI calls it before writing the
// deploy-state marker so a refusal cannot plant a marker that would
// bypass the guard on the next invocation.
func (p *Provisioner) GuardSetup(cfg *config.Config, opts SetupOpts) error {
	setupOpts := setup.NewOptions(cfg, p.projectRoot)
	if !system.DirExists(setupOpts.WorkDir) {
		return nil
	}
	return p.guardLiveCluster(cfg, opts)
}

// guardLiveCluster returns a *errtypes.ConfigError when the terraform env
// state has resources — the authoritative live-cluster signal. A
// cluster-config/auth directory alone is mid-setup debris (written by
// StepGenerateIgnition before any VM exists) and stays wipeable. Only
// FreshDeploy bypasses the guard: ResumeInProgress never unlocks the wipe,
// because populated state contradicts the setup-phase marker it vouches
// for — trusting the marker there would wipe material live VMs depend on.
//
// Callers that set FreshDeploy=true accept credential loss: cluster-config/auth
// (kubeadmin-password, kubeconfig) is wiped with no backup.
func (p *Provisioner) guardLiveCluster(cfg *config.Config, opts SetupOpts) error {
	if opts.FreshDeploy {
		return nil
	}
	tfEnv := phase.GetTerraformEnv(cfg)
	envDir := phase.TerraformEnvDir(p.projectRoot, tfEnv)
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

// PostInstall runs the postinstall phase: kube-vip verification, production
// DNS cutover, bootstrap cleanup. Returns the result alongside per-step
// records. keepRedHatCatalogs mirrors the deploy --keep-redhat-catalogs flag,
// skipping the step that disables subscription-gated OperatorHub catalog
// sources and the InsightsDisabled alert.
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

// ResumePostInstall runs the postinstall phase for a deploy interrupted
// during postinstall. The cluster is installed and its VMs are live, so
// Install is not re-run — a terraform re-apply after bootstrap cleanup would
// recreate the bootstrap VM against a running control plane. Only KUBECONFIG,
// the process-local executor state Install normally establishes, is re-armed;
// a missing kubeconfig fails fast before any postinstall step runs.
func (p *Provisioner) ResumePostInstall(ctx context.Context, cfg *config.Config, keepRedHatCatalogs bool) (*postinstall.Result, []distribution.StepResult, error) {
	installPhase := install.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	clusterDir := phase.ClusterConfigDir(filepath.Join(p.projectRoot, phase.WorkDirName))
	if err := installPhase.SetupKubeconfig(ctx, clusterDir); err != nil {
		return nil, nil, &errtypes.ClusterError{
			Msg: "cannot resume postinstall: cluster kubeconfig unavailable; " +
				"run 'okdctl destroy' then re-deploy, or re-run with --fresh to restart from scratch (credentials will be lost)",
			Err: err,
		}
	}
	return p.PostInstall(ctx, cfg, keepRedHatCatalogs)
}

// DeployStep is a single step's identity for dry-run listings and the live
// checklist — ID, display name, and the phase (setup/install/postinstall)
// that owns it, no executable body.
type DeployStep struct {
	ID    distribution.StepID
	Name  string
	Phase string
}

// Deploy phase labels carried on DeployStep.Phase. They match the deploy
// package's on-disk marker phase names so a resume can filter the plan to the
// phases it will actually run.
const (
	PhaseSetup       = "setup"
	PhaseInstall     = "install"
	PhasePostInstall = "postinstall"
)

// DeploySteps returns the ordered ID+Name for every step the setup, install,
// and postinstall phases execute for cfg, derived from the same StepDefs
// Setup/Install/PostInstall feed into BuildSteps — so a step added to (or
// reordered in) a phase's xSteps() method cannot drift from this listing.
func (p *Provisioner) DeploySteps(cfg *config.Config) []DeployStep {
	setupPhase := setup.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	setupPhase.BinDir = config.ResolveBinDir(cfg)
	setupOpts := setup.NewOptions(cfg, p.projectRoot)

	installPhase := install.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	installOpts := install.NewOptions(cfg, p.projectRoot)

	postPhase := postinstall.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	postOpts := postinstall.NewOptions(cfg, p.projectRoot)

	var out []DeployStep
	appendPhase := func(phaseName string, defs []distribution.StepDef) {
		for _, d := range defs {
			out = append(out, DeployStep{ID: d.ID, Name: d.Name, Phase: phaseName})
		}
	}
	appendPhase(PhaseSetup, setupPhase.StepDefs(cfg, &setupOpts))
	appendPhase(PhaseInstall, installPhase.StepDefs(cfg, &installOpts))
	appendPhase(PhasePostInstall, postPhase.StepDefs(cfg, &postOpts))
	return out
}

// UpdateIngress re-points haproxy at a fresh set of backend nodes without
// re-running the full postinstall phase. Used by the update-ingress CLI verb.
func (p *Provisioner) UpdateIngress(ctx context.Context, cfg *config.Config, opts postinstall.UpdateIngressOptions) (*postinstall.UpdateIngressResult, error) {
	postPhase := postinstall.New(
		phase.WithExecutor(p.executor),
		phase.WithLogger(p.logger),
	)
	opts.WorkDir = resolveIngressWorkDir(p.projectRoot, opts.WorkDir)
	return postPhase.UpdateIngress(ctx, cfg, opts)
}

// resolveIngressWorkDir defaults an empty WorkDir to the okd-install
// directory under projectRoot. UpdateIngressOptions.WorkDir is the parent
// of cluster-config/, NOT the project root — passing projectRoot here is
// the regression that pointed RemoveHAProxy's kubeconfig-CA pre-flight at
// a path that never exists.
func resolveIngressWorkDir(projectRoot, workDir string) string {
	if workDir == "" {
		return filepath.Join(projectRoot, phase.WorkDirName)
	}
	return workDir
}

// ZeroizeEnv delegates to the underlying executor's ZeroizeEnv, bounding
// the lifetime of plaintext credential strings. Call via defer after all
// phases complete. Kept as credential-lifecycle scaffolding; field owner
// is executor.Executor.ZeroizeEnv.
func (p *Provisioner) ZeroizeEnv() {
	if p.executor == nil {
		return
	}
	p.executor.ZeroizeEnv()
}

// Destroy tears down the cluster and its infrastructure. Callers build opts
// via destroy.NewOptions(cfg, projectRoot) and override the fields they
// need — there is no separate CLI-facing options type to keep in sync.
func (p *Provisioner) Destroy(ctx context.Context, cfg *config.Config, opts *destroy.Options) ([]distribution.StepResult, error) {
	destroyPhase := destroy.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	return destroyPhase.Execute(ctx, cfg, opts)
}

// Cleanup removes local cluster artifacts without touching infrastructure.
// Callers build opts via cleanup.NewOptions(cfg, projectRoot, kind) and
// override the fields they need — the same pass-through contract as
// Destroy. Setup's internal pre-deploy cleanup does not route through here;
// it wires its own WorkOnly run with the FreshDeploy consent applied.
func (p *Provisioner) Cleanup(ctx context.Context, opts *cleanup.Options) error {
	cleanupPhase := cleanup.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	return cleanupPhase.Execute(ctx, opts)
}
