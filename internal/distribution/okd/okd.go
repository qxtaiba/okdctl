// Package okd implements the OKD/OpenShift provisioner for Proxmox.
//
// # Architecture
//
// The Provisioner is a thin coordinator that delegates to phase-specific packages:
//
//	okd/
//	├── okd.go           # Provisioner (coordinator) - this file
//	├── setup/           # Setup phase (artifacts, ISOs, HAProxy, DNS)
//	├── install/         # Install phase (Terraform, bootstrap, CSR approval)
//	├── postinstall/     # Post-install phase (verification, MetalLB, ingress)
//	├── destroy/         # Destroy phase (Terraform destroy, cleanup)
//	├── cleanup/         # Cleanup utilities
//	└── dns/             # DNS utilities (dnsmasq)
//
// Each phase package contains its own Options, Phase struct, and Execute() method.
// The Provisioner simply creates Phase instances and delegates execution.
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
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

type Logger = logging.Logger

// Minimum resource requirements for OKD control plane nodes.
const (
	MinControlPlaneMemoryMB = 8192
	MinControlPlaneCPUs     = 4
	MinControlPlaneDiskGB   = 50
)

// Provisioner coordinates OKD cluster lifecycle operations.
// It delegates actual work to phase-specific packages.
type Provisioner struct {
	version     string
	projectRoot string
	executor    *executor.Executor
	logger      Logger
}

// ProvisionerOption configures a Provisioner.
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

// WithEnv sets environment variables for command execution.
// These are passed to the executor for all subprocess calls.
// Use this to pass credentials without modifying global process environment.
func WithEnv(env []string) ProvisionerOption {
	return func(p *Provisioner) {
		if p.executor == nil {
			p.executor = executor.New(executor.WithEnv(env))
		} else {
			// Append to existing executor's environment
			p.executor.Env = append(p.executor.Env, env...)
		}
	}
}

// New creates a new OKD provisioner with optional configuration.
func New(version string, opts ...ProvisionerOption) *Provisioner {
	projectRoot, _ := os.Getwd()

	p := &Provisioner{
		version:     version,
		projectRoot: projectRoot,
		logger:      logging.NoopLogger(),
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.executor == nil {
		p.executor = executor.New()
	}

	return p
}

// ════════════════════════════════════════════════════════════════════════════════
// VALIDATION
// ════════════════════════════════════════════════════════════════════════════════

// Validate checks if the configuration is valid for OKD.
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

// ════════════════════════════════════════════════════════════════════════════════
// LIFECYCLE PHASES - Thin delegation to phase packages
// ════════════════════════════════════════════════════════════════════════════════

// Prepare performs pre-installation setup for OKD.
// Cleans up previous artifacts first, then delegates to setup.Phase.Execute().
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

// Install installs OKD on the provisioned infrastructure.
// Delegates to install.Phase.Execute().
func (p *Provisioner) Install(ctx context.Context, cfg *config.Config, opts install.Options) error {
	phase := install.New(p.executor, p.logger, p.version)
	return phase.Execute(ctx, cfg, opts)
}

// Configure applies post-installation configuration.
// Delegates to postinstall.Phase.Execute().
func (p *Provisioner) Configure(ctx context.Context, cfg *config.Config) (*postinstall.Result, error) {
	phase := postinstall.New(p.executor, p.logger, p.version)
	opts := postinstall.NewOptions(cfg, p.projectRoot)
	return phase.Execute(ctx, cfg, opts)
}

// DestroyOptions configures the destroy operation.
type DestroyOptions struct {
	// RemovePackages removes system packages installed during setup.
	RemovePackages bool
}

// Destroy removes the OKD installation.
// Delegates to destroy.Phase.Execute().
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
