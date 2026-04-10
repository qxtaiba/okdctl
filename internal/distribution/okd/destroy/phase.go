// Package destroy provides the destroy phase implementation for OKD cluster teardown.
// It handles Terraform destroy, cleanup of configuration files, and service teardown.
package destroy

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
)

// Options configures the destroy phase.
type Options struct {
	phase.BaseOptions

	// AutoApprove skips Terraform confirmation prompts.
	AutoApprove bool

	// CleanupKind specifies what to clean up after destruction.
	CleanupKind cleanup.Kind

	// Force skips confirmation prompts.
	Force bool

	// Parallelism controls Terraform parallelism.
	Parallelism int

	// SkipTerraform skips Terraform destroy (useful for cleanup-only operations).
	SkipTerraform bool

	// SkipCleanup skips file cleanup after Terraform destroy.
	SkipCleanup bool

	// SkipFirewall skips firewall rule cleanup.
	SkipFirewall bool

	// RemovePackages removes system packages installed during setup.
	// When true, packages like haproxy, httpd, dnsmasq, etc. will be uninstalled.
	RemovePackages bool
}

// NewOptions returns destroy options derived from config with sensible defaults.
func NewOptions(cfg *config.Config, projectRoot string) Options {
	return Options{
		BaseOptions: phase.BaseOptions{
			ProjectRoot:  projectRoot,
			WorkDir:      filepath.Join(projectRoot, "okd-install"),
			TerraformEnv: phase.GetTerraformEnv(cfg),
		},
		AutoApprove: cfg.Deployment.AutoApprove,
		CleanupKind: cleanup.Full,
	}
}

// Phase coordinates the destroy phase execution.
type Phase struct {
	phase.BasePhase
}

// New creates a new destroy phase coordinator.
func New(exec *executor.Executor, logger *slog.Logger, version string) *Phase {
	return &Phase{
		BasePhase: phase.NewBasePhase(exec, logger, version),
	}
}

// Execute tears down the cluster. User confirmation is the CLI layer's
// responsibility; by the time Execute runs, opts.Force is expected to
// be true.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) error {
	p.Log.Info("destroy: starting cluster teardown")
	p.Log.Warn("destroy: this will permanently remove all vms and generated files")

	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.destroySteps(cfg, opts))...)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return err
	}

	return nil
}
