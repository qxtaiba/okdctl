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
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Provisioner orchestrates the OKD distribution's phase packages (setup,
// install, postinstall, destroy). Construct via New with functional options;
// the zero value is not usable.
type Provisioner struct {
	version     string
	projectRoot string
	executor    *executor.Executor
	pendingEnv  []string
	logger      *slog.Logger
	recorder    distribution.MetricsRecorder
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

// WithEnv passes environment variables to the executor for all subprocess calls,
// avoiding modification of the global process environment.
func WithEnv(env []string) ProvisionerOption {
	return func(p *Provisioner) {
		p.pendingEnv = append(p.pendingEnv, env...)
	}
}

// New constructs a Provisioner for the given okdctl version with options
// applied in order. Normalizes a nil logger to NopLogger and builds the
// executor once after all options are applied so WithEnv/WithLogger
// ordering does not matter.
func New(version string, opts ...ProvisionerOption) *Provisioner {
	projectRoot, _ := os.Getwd()

	p := &Provisioner{
		version:     version,
		projectRoot: projectRoot,
		logger:      logutil.NopLogger,
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

// Prepare cleans up previous artifacts and runs the setup phase.
func (p *Provisioner) Prepare(ctx context.Context, cfg *config.Config) ([]distribution.StepResult, error) {
	opts := setup.NewOptions(cfg, p.projectRoot)

	if system.DirExists(opts.WorkDir) {
		p.logger.Info("setup: cleaning up previous artifacts")
		cleanupOpts := &cleanup.Options{
			BaseOptions: phase.BaseOptions{
				WorkDir:     opts.WorkDir,
				ProjectRoot: p.projectRoot,
			},
			Kind:           cleanup.WorkOnly,
			HTTPServerRoot: cfg.HTTPServer.Root,
			Logger:         p.logger,
		}
		if err := cleanup.New(p.executor, p.logger, p.version).Execute(ctx, cleanupOpts); err != nil {
			p.logger.Warn("cleanup: pre-deploy artifact removal incomplete", "phase", "prepare", "err", err)
		}
	}

	setupPhase := setup.New(p.executor, p.logger, p.version)
	setupPhase.Recorder = p.recorder
	setupPhase.BinDir = phase.ResolveBinDir(cfg)
	return setupPhase.Execute(ctx, cfg, &opts)
}

// Install runs the install phase: ignition delivery, bootstrap wait, and
// install-complete monitor. Must be called after Prepare.
func (p *Provisioner) Install(ctx context.Context, cfg *config.Config, opts *install.Options) ([]distribution.StepResult, error) {
	installPhase := install.New(p.executor, p.logger, p.version)
	installPhase.Recorder = p.recorder
	return installPhase.Execute(ctx, cfg, opts)
}

// Configure runs the postinstall phase: kube-vip verification, production
// DNS cutover, bootstrap cleanup. Returns the result alongside per-step records.
func (p *Provisioner) Configure(ctx context.Context, cfg *config.Config) (*postinstall.Result, []distribution.StepResult, error) {
	postPhase := postinstall.New(p.executor, p.logger, p.version)
	postPhase.Recorder = p.recorder
	opts := postinstall.NewOptions(cfg, p.projectRoot)
	return postPhase.Execute(ctx, cfg, &opts)
}

// UpdateIngress re-points haproxy at a fresh set of backend nodes without
// re-running the full postinstall phase. Used by the update-ingress CLI verb.
func (p *Provisioner) UpdateIngress(ctx context.Context, cfg *config.Config, opts postinstall.UpdateIngressOptions) (*postinstall.UpdateIngressResult, error) {
	postPhase := postinstall.New(p.executor, p.logger, p.version)
	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Join(p.projectRoot, "okd-install")
	}
	return postPhase.UpdateIngress(ctx, cfg, opts)
}

// DestroyOpts configures a Provisioner.Destroy run. Zero-value runs a
// full teardown; Skip* flags carve out individual steps so operators
// can retry a partial run (e.g. SkipTerraform=true to re-run just the
// file cleanup after a successful terraform destroy).
type DestroyOpts struct {
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

// Destroy tears down the cluster and its infrastructure.
func (p *Provisioner) Destroy(ctx context.Context, cfg *config.Config, opts DestroyOpts) ([]distribution.StepResult, error) {
	destroyPhase := destroy.New(p.executor, p.logger, p.version)
	destroyOpts := destroy.NewOptions(cfg, p.projectRoot)
	destroyOpts.AutoApprove = true
	destroyOpts.Force = true
	destroyOpts.RemovePackages = opts.RemovePackages
	destroyOpts.KeepISOs = opts.KeepISOs
	destroyOpts.SkipTerraform = opts.SkipTerraform
	destroyOpts.SkipCleanup = opts.SkipCleanup
	destroyOpts.SkipFirewall = opts.SkipFirewall
	destroyOpts.TerraformTargets = opts.TerraformTargets

	return destroyPhase.Execute(ctx, cfg, &destroyOpts)
}
