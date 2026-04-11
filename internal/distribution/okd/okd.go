// Package okd implements the OKD/OpenShift provisioner for Proxmox.
// The Provisioner delegates to phase-specific packages (setup, install, postinstall, destroy).
package okd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/destroy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/install"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/utils/logutil"
	"github.com/qxtaiba/okdctl/internal/utils/system"
)

const (
	MinControlPlaneMemoryMB = 8192
	MinControlPlaneCPUs     = 4
	MinControlPlaneDiskGB   = 50
)

type Provisioner struct {
	version     string
	projectRoot string
	executor    *executor.Executor
	logger      *slog.Logger
}

type ProvisionerOption func(*Provisioner)

func WithProjectRoot(projectRoot string) ProvisionerOption {
	return func(p *Provisioner) {
		p.projectRoot = projectRoot
	}
}

func WithLogger(l *slog.Logger) ProvisionerOption {
	return func(p *Provisioner) {
		p.logger = l
	}
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

func (p *Provisioner) Validate(cfg *config.Config) error {
	if cfg.Distribution.Type != config.DistributionOKD {
		return fmt.Errorf("invalid distribution type: expected okd, got %s", cfg.Distribution.Type)
	}

	if cfg.Topology.ControlPlane.Memory < MinControlPlaneMemoryMB {
		return fmt.Errorf("okd requires at least %dMB RAM for control plane nodes", MinControlPlaneMemoryMB)
	}

	if cfg.Topology.ControlPlane.CPU < MinControlPlaneCPUs {
		return fmt.Errorf("okd requires at least %d vCPUs for control plane nodes", MinControlPlaneCPUs)
	}

	if cfg.Topology.ControlPlane.Disk < MinControlPlaneDiskGB {
		return fmt.Errorf("okd requires at least %dGB disk for control plane nodes", MinControlPlaneDiskGB)
	}

	return nil
}

// Prepare cleans up previous artifacts and runs the setup phase.
func (p *Provisioner) Prepare(ctx context.Context, cfg *config.Config) error {
	opts := setup.DefaultOptions(p.projectRoot)

	if system.DirExists(opts.WorkDir) {
		p.logger.Info("setup: cleaning up previous artifacts")
		cleanupOpts := &cleanup.Options{
			Kind:           cleanup.WorkOnly,
			WorkDir:        opts.WorkDir,
			ProjectRoot:    p.projectRoot,
			HTTPServerRoot: cfg.HTTPServer.Root,
			Logger:         p.logger,
		}
		if err := cleanup.Execute(ctx, cleanupOpts); err != nil {
			p.logger.Warn(fmt.Sprintf("cleanup warning: %v", err))
		}
	}

	phase := setup.New(p.executor, p.logger, p.version)
	return phase.Execute(ctx, cfg, &opts)
}

func (p *Provisioner) Install(ctx context.Context, cfg *config.Config, opts *install.Options) error {
	phase := install.New(p.executor, p.logger, p.version)
	return phase.Execute(ctx, cfg, opts)
}

func (p *Provisioner) Configure(ctx context.Context, cfg *config.Config) (*postinstall.Result, error) {
	phase := postinstall.New(p.executor, p.logger, p.version)
	opts := postinstall.NewOptions(cfg, p.projectRoot)
	return phase.Execute(ctx, cfg, &opts)
}

func (p *Provisioner) UpdateIngress(ctx context.Context, cfg *config.Config, opts postinstall.UpdateIngressOptions) (*postinstall.UpdateIngressResult, error) {
	phase := postinstall.New(p.executor, p.logger, p.version)
	return phase.UpdateIngress(ctx, cfg, opts)
}

func (p *Provisioner) Destroy(ctx context.Context, cfg *config.Config, removePackages bool) error {
	phase := destroy.New(p.executor, p.logger, p.version)
	opts := destroy.NewOptions(cfg, p.projectRoot)
	opts.AutoApprove = true
	opts.Force = true
	opts.RemovePackages = removePackages

	return phase.Execute(ctx, cfg, &opts)
}
