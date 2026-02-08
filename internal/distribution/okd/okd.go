// Package okd implements the OKD/OpenShift provisioner for Proxmox.
// The Provisioner delegates to phase-specific packages (setup, install, postinstall, destroy).
package okd

import (
	"context"
	"fmt"
	"os"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/destroy"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/install"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/postinstall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/setup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

type Logger = utils.Logger

const (
	MinControlPlaneMemoryMB = 8192
	MinControlPlaneCPUs     = 4
	MinControlPlaneDiskGB   = 50
)

type Provisioner struct {
	version     string
	projectRoot string
	executor    *executor.Executor
	logger      Logger
}

type ProvisionerOption func(*Provisioner)

func WithProjectRoot(projectRoot string) ProvisionerOption {
	return func(p *Provisioner) {
		p.projectRoot = projectRoot
	}
}

func WithLogger(l Logger) ProvisionerOption {
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
		logger:      utils.NoopLogger(),
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.executor == nil {
		p.executor = executor.New()
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
		cleanupOpts := cleanup.Options{
			Type:           cleanup.TypeWorkOnly,
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
	return phase.Execute(ctx, cfg, opts)
}

func (p *Provisioner) Install(ctx context.Context, cfg *config.Config, opts install.Options) error {
	phase := install.New(p.executor, p.logger, p.version)
	return phase.Execute(ctx, cfg, opts)
}

func (p *Provisioner) Configure(ctx context.Context, cfg *config.Config) (*postinstall.Result, error) {
	phase := postinstall.New(p.executor, p.logger, p.version)
	opts := postinstall.NewOptions(cfg, p.projectRoot)
	return phase.Execute(ctx, cfg, opts)
}

type UpdateIngressOptions struct {
	RemoveHAProxy bool
}

type DestroyOptions struct {
	RemovePackages bool
}

func (p *Provisioner) UpdateIngress(ctx context.Context, cfg *config.Config, opts *UpdateIngressOptions) (*postinstall.UpdateIngressResult, error) {
	phase := postinstall.New(p.executor, p.logger, p.version)
	piOpts := postinstall.UpdateIngressOptions{}
	if opts != nil {
		piOpts.RemoveHAProxy = opts.RemoveHAProxy
	}
	return phase.UpdateIngress(ctx, cfg, piOpts)
}

func (p *Provisioner) Destroy(ctx context.Context, cfg *config.Config, destroyOpts *DestroyOptions) error {
	phase := destroy.New(p.executor, p.logger, p.version)
	opts := destroy.NewOptions(cfg, p.projectRoot)
	opts.AutoApprove = true
	opts.Force = true

	if destroyOpts != nil {
		opts.RemovePackages = destroyOpts.RemovePackages
	}

	return phase.Execute(ctx, cfg, opts)
}
