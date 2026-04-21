// Package destroy provides the destroy phase implementation for OKD cluster teardown.
// It handles Terraform destroy, cleanup of configuration files, and service teardown.
package destroy

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// Options configures the destroy phase.
type Options struct {
	phase.BaseOptions

	AutoApprove bool
	CleanupKind cleanup.Kind
	Force       bool
	Parallelism int

	SkipTerraform bool
	SkipCleanup   bool
	SkipFirewall  bool

	// RemovePackages removes system packages installed during setup.
	// When true, packages like haproxy, httpd, dnsmasq, etc. will be uninstalled.
	RemovePackages bool

	// KeepISOs skips removal of fedora-coreos-*.iso from the Proxmox host.
	// Useful when chaining a destroy with an immediate re-deploy.
	KeepISOs bool
}

// NewOptions builds the default destroy Options from cfg and projectRoot.
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

// Phase drives the destroy flow: terraform destroy, host-file cleanup,
// firewall teardown, and ISO removal.
type Phase struct {
	phase.BasePhase
}

// New constructs a destroy Phase bound to exec/logger and the okdctl
// version tag.
func New(exec *executor.Executor, logger *slog.Logger, version string) *Phase {
	return &Phase{
		BasePhase: phase.NewBasePhase(version, phase.WithExecutor(exec), phase.WithLogger(logger)),
	}
}

// Execute tears down the cluster. User confirmation is the CLI layer's
// responsibility; by the time Execute runs, opts.Force is expected to
// be true.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) ([]distribution.StepResult, error) {
	p.Log.Info("destroy: starting cluster teardown")
	p.Log.Warn("destroy: this will permanently remove all vms and generated files")

	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.destroySteps(cfg, opts))...)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return orchestrator.Results(), err
	}

	return orchestrator.Results(), nil
}
