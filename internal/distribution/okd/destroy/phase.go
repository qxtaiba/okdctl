// Package destroy provides the destroy phase implementation for OKD cluster teardown.
// It handles Terraform destroy, cleanup of configuration files, and service teardown.
package destroy

import (
	"context"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
)

// Options configures the destroy phase.
type Options struct {
	phase.BaseOptions

	AutoApprove bool
	CleanupKind cleanup.Kind
	Parallelism int

	SkipTerraform bool
	SkipCleanup   bool
	SkipFirewall  bool

	// TerraformTargets limits terraform destroy to specific resource addresses.
	// When non-empty only the named resources (and their dependents) are destroyed.
	TerraformTargets []string

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

// New constructs a destroy Phase with the given options.
func New(opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(opts...)
	bp.Log = bp.Log.With("phase", "destroy")
	return &Phase{BasePhase: bp}
}

// Execute tears down the cluster. User confirmation is the CLI layer's
// responsibility before this is called.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) ([]distribution.StepResult, error) {
	p.Log.Info("destroy: starting cluster teardown")
	p.Log.Warn("destroy: this will permanently remove all vms and generated files")

	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.destroySteps(ctx, cfg, opts))...)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return orchestrator.Results(), err
	}

	return orchestrator.Results(), nil
}
