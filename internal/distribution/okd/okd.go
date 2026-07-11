// Package okd implements the OKD/OpenShift provisioner for Proxmox.
// The Provisioner delegates to phase-specific packages (setup, install, postinstall, destroy).
package okd

import (
	"context"
	"fmt"
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
// install, postinstall, destroy). Construct via New with functional options;
// the zero value is not usable.
type Provisioner struct {
	projectRoot string
	executor    *executor.Executor
	pendingEnv  []string
	logger      *slog.Logger
	recorder    distribution.MetricsRecorder
	reporter    logutil.ProgressReporter
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
	}

	for _, opt := range opts {
		opt(p)
	}

	execOpts := []executor.Option{executor.WithLogger(p.logger)}
	if len(p.pendingEnv) > 0 {
		execOpts = append(execOpts, executor.WithEnv(p.pendingEnv))
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

// PrepareOpts configures a Provisioner.Prepare run. FreshDeploy permits
// wiping a work directory that contains live cluster state (terraform
// resources or cluster-config/auth); without it Prepare returns an error
// so the operator is forced to destroy the cluster first.
//
// ResumeInProgress signals that the CLI's deploy-state marker records a
// prepare-phase interruption for this cluster. A prepare run applies
// nothing to Proxmox, so populated terraform state alongside such a
// marker is a contradiction (a failed install-marker write, or a --fresh
// run interrupted over an older cluster); the guard then refuses with a
// contradiction diagnostic instead of unlocking the wipe. Markers
// recording an install or configure interruption must never reach
// Prepare: the CLI routes those resumes past the wipe entirely
// (cli.resolveResumePhase) because the work directory then holds
// identity material live VMs depend on.
type PrepareOpts struct {
	FreshDeploy      bool
	ResumeInProgress bool
}

// Prepare cleans up previous artifacts and runs the setup phase. It refuses
// to wipe a work directory that appears to belong to a live cluster unless
// opts.FreshDeploy is true; pass --fresh at the CLI to opt in.
func (p *Provisioner) Prepare(ctx context.Context, cfg *config.Config, opts PrepareOpts) ([]distribution.StepResult, error) {
	setupOpts := setup.NewOptions(cfg, p.projectRoot)

	if system.DirExists(setupOpts.WorkDir) {
		if err := p.guardLiveCluster(cfg, opts); err != nil {
			return nil, err
		}
		p.logger.Info("setup: cleaning up previous artifacts")
		cleanupOpts := &cleanup.Options{
			BaseOptions: phase.BaseOptions{
				WorkDir:     setupOpts.WorkDir,
				ProjectRoot: p.projectRoot,
			},
			Kind:           cleanup.WorkOnly,
			HTTPServerRoot: cfg.HTTPServer.Root,
		}
		if err := cleanup.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger)).Execute(ctx, cleanupOpts); err != nil {
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

// GuardPrepare reports whether Prepare would refuse to wipe the work
// directory, without side effects. The CLI calls it before writing the
// deploy-state marker so a refusal cannot plant a marker that would
// bypass the guard on the next invocation.
func (p *Provisioner) GuardPrepare(cfg *config.Config, opts PrepareOpts) error {
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
// because populated state contradicts the prepare-phase marker it vouches
// for — trusting the marker there would wipe material live VMs depend on.
//
// Callers that set FreshDeploy=true accept credential loss: cluster-config/auth
// (kubeadmin-password, kubeconfig) is wiped with no backup.
func (p *Provisioner) guardLiveCluster(cfg *config.Config, opts PrepareOpts) error {
	if opts.FreshDeploy {
		return nil
	}
	tfEnv := phase.GetTerraformEnv(cfg)
	envDir := filepath.Join(p.projectRoot, "infrastructure", "terraform", "environments", tfEnv)
	tf := terraform.New(envDir, terraform.WithLogger(p.logger))
	if !tf.HasState() {
		return nil
	}
	if opts.ResumeInProgress {
		return &errtypes.ConfigError{
			Msg: "deploy-state marker records a prepare-phase interruption but terraform state has resources — " +
				"a prepare run applies nothing, so the marker cannot be trusted; " +
				"run 'okdctl destroy' first, or pass --fresh to force-wipe (credentials will be lost)",
		}
	}
	return &errtypes.ConfigError{
		Msg: "terraform state has resources — the work directory belongs to a live cluster; " +
			"run 'okdctl destroy' first, or pass --fresh to force-wipe (credentials will be lost)",
	}
}

// Install runs the install phase: ignition delivery, bootstrap wait, and
// install-complete monitor. Must be called after Prepare.
func (p *Provisioner) Install(ctx context.Context, cfg *config.Config, opts *install.Options) ([]distribution.StepResult, error) {
	installPhase := install.New(
		phase.WithExecutor(p.executor),
		phase.WithLogger(p.logger),
		phase.WithRecorder(p.recorder),
		phase.WithReporter(p.reporter),
	)
	return installPhase.Execute(ctx, cfg, opts)
}

// Configure runs the postinstall phase: kube-vip verification, production
// DNS cutover, bootstrap cleanup. Returns the result alongside per-step records.
func (p *Provisioner) Configure(ctx context.Context, cfg *config.Config) (*postinstall.Result, []distribution.StepResult, error) {
	postPhase := postinstall.New(
		phase.WithExecutor(p.executor),
		phase.WithLogger(p.logger),
		phase.WithRecorder(p.recorder),
	)
	opts := postinstall.NewOptions(cfg, p.projectRoot)
	return postPhase.Execute(ctx, cfg, &opts)
}

// ResumeConfigure runs the postinstall phase for a deploy interrupted during
// configure. The cluster is installed and its VMs are live, so Install is
// not re-run — a terraform re-apply after bootstrap cleanup would recreate
// the bootstrap VM against a running control plane. Only KUBECONFIG, the
// process-local executor state Install normally establishes, is re-armed;
// a missing kubeconfig fails fast before any postinstall step runs.
func (p *Provisioner) ResumeConfigure(ctx context.Context, cfg *config.Config) (*postinstall.Result, []distribution.StepResult, error) {
	installPhase := install.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	clusterDir := phase.ClusterConfigDir(filepath.Join(p.projectRoot, "okd-install"))
	if err := installPhase.SetupKubeconfig(ctx, clusterDir); err != nil {
		return nil, nil, &errtypes.ClusterError{
			Msg: "cannot resume configure: cluster kubeconfig unavailable; " +
				"run 'okdctl destroy' then re-deploy, or re-run with --fresh to restart from scratch (credentials will be lost)",
			Err: err,
		}
	}
	return p.Configure(ctx, cfg)
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
		return filepath.Join(projectRoot, "okd-install")
	}
	return workDir
}

// DestroyOpts configures a Provisioner.Destroy run. Zero-value runs a
// full teardown; Skip* flags carve out individual steps so operators
// can retry a partial run (e.g. SkipTerraform=true to re-run just the
// file cleanup after a successful terraform destroy).
type DestroyOpts struct {
	AutoApprove    bool
	RemovePackages bool
	KeepISOs       bool
	SkipTerraform  bool
	SkipCleanup    bool
	SkipFirewall   bool
	// TerraformTargets, when non-empty, limits the terraform destroy to
	// the named resource addresses. Each entry must pass the CLI-layer
	// allowlist before reaching this struct.
	TerraformTargets []string
}

// ZeroizeEnv delegates to the underlying executor's ZeroizeEnv, bounding
// the lifetime of plaintext credential strings. Call via defer after all
// phases complete. Kept as credential-lifecycle scaffolding (api:c287d5c0);
// field owner is executor.Executor.ZeroizeEnv.
func (p *Provisioner) ZeroizeEnv() {
	if p.executor == nil {
		return
	}
	p.executor.ZeroizeEnv()
}

// Destroy tears down the cluster and its infrastructure.
func (p *Provisioner) Destroy(ctx context.Context, cfg *config.Config, opts DestroyOpts) ([]distribution.StepResult, error) {
	destroyPhase := destroy.New(phase.WithExecutor(p.executor), phase.WithLogger(p.logger))
	destroyOpts := destroy.NewOptions(cfg, p.projectRoot)
	destroyOpts.AutoApprove = opts.AutoApprove
	destroyOpts.RemovePackages = opts.RemovePackages
	destroyOpts.KeepISOs = opts.KeepISOs
	destroyOpts.SkipTerraform = opts.SkipTerraform
	destroyOpts.SkipCleanup = opts.SkipCleanup
	destroyOpts.SkipFirewall = opts.SkipFirewall
	destroyOpts.TerraformTargets = opts.TerraformTargets

	return destroyPhase.Execute(ctx, cfg, &destroyOpts)
}
