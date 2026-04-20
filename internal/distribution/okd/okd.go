// Package okd implements the OKD/OpenShift provisioner for Proxmox.
// The Provisioner delegates to phase-specific packages (setup, install, postinstall, destroy).
package okd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/destroy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
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
		if p.executor == nil {
			p.executor = executor.New(executor.WithEnv(env))
		} else {
			p.executor.Env = append(p.executor.Env, env...)
		}
	}
}

// New constructs a Provisioner for the given okdctl version with options
// applied in order. Normalizes a nil logger to NopLogger and guarantees an
// executor is attached even when WithEnv did not allocate one.
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

	if p.executor == nil {
		p.executor = executor.New(executor.WithLogger(p.logger))
	} else {
		// WithEnv may have constructed the executor before WithLogger was
		// applied; ensure the final logger is attached either way.
		executor.WithLogger(p.logger)(p.executor)
	}

	return p
}

// Validate checks the distribution type and delegates resource-minimum
// validation to config-side ValidateOKDConfig so a single source of truth
// is enforced (config-load time and provisioner entry both hit it).
func (p *Provisioner) Validate(cfg *config.Config) error {
	if cfg.Distribution.Type != config.DistributionOKD {
		return fmt.Errorf("invalid distribution type: expected okd, got %s", cfg.Distribution.Type)
	}
	return nil
}

// Prepare cleans up previous artifacts and runs the setup phase.
func (p *Provisioner) Prepare(ctx context.Context, cfg *config.Config) ([]distribution.StepResult, error) {
	opts := setup.DefaultOptions(p.projectRoot)

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
		if err := cleanup.Execute(ctx, cleanupOpts); err != nil {
			p.logger.Warn("cleanup warning", "err", err)
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
	return postPhase.UpdateIngress(ctx, cfg, opts)
}

// Destroy tears down the cluster and its infrastructure. removePackages=true
// also uninstalls host packages (haproxy, dnsmasq, httpd); keepISOs=true
// preserves uploaded FCOS ISOs so a subsequent install can skip the upload.
func (p *Provisioner) Destroy(ctx context.Context, cfg *config.Config, removePackages, keepISOs bool) ([]distribution.StepResult, error) {
	destroyPhase := destroy.New(p.executor, p.logger, p.version)
	destroyOpts := destroy.NewOptions(cfg, p.projectRoot)
	destroyOpts.AutoApprove = true
	destroyOpts.Force = true
	destroyOpts.RemovePackages = removePackages
	destroyOpts.KeepISOs = keepISOs

	return destroyPhase.Execute(ctx, cfg, &destroyOpts)
}
