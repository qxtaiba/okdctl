// Package destroy provides the destroy phase implementation for OKD cluster teardown.
// It handles Terraform destroy, cleanup of configuration files, and service teardown.
package destroy

import (
	"context"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// Options configures the destroy phase.
type Options struct {
	paths.BaseOptions

	// AutoApprove skips Terraform confirmation prompts.
	AutoApprove bool

	// CleanupType specifies what to clean up after destruction.
	CleanupType cleanup.Type

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
		BaseOptions: paths.BaseOptions{
			ProjectRoot:  projectRoot,
			WorkDir:      filepath.Join(projectRoot, "okd-install"),
			TerraformEnv: paths.GetTerraformEnv(cfg),
		},
		AutoApprove: cfg.Deployment.AutoApprove,
		CleanupType: cleanup.TypeFull,
	}
}

// Phase coordinates the destroy phase execution.
type Phase struct {
	paths.BasePhase
}

// New creates a new destroy phase coordinator.
func New(exec *executor.Executor, logger utils.Logger, version string) *Phase {
	return &Phase{
		BasePhase: paths.NewBasePhase(exec, logger, version),
	}
}

// Execute destroys the OKD cluster infrastructure using the step orchestrator.
// This is the main entry point for the destroy phase.
// NOTE: User confirmation is handled by the CLI layer before calling this method.
// The Force option is expected to be true when called from CLI (which has already confirmed).
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts Options) error {
	p.Log.Info("destroy: starting cluster teardown")
	p.Log.Warn("destroy: this will permanently remove all vms and generated files")

	orchestrator := distribution.NewOrchestrator(
		p.newDestroyInfraStep(cfg, opts),
		p.newCleanupFilesStep(cfg, opts),
		p.newCleanupFirewallStep(opts),
		p.newPrintSummaryStep(opts),
	)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return err
	}

	return nil
}
