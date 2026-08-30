// Package destroy provides the destroy phase implementation for OKD cluster teardown.
// It handles Terraform destroy, cleanup of configuration files, and service teardown.
package destroy

import (
	"context"

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

	// TerraformTargets, when non-empty, limits destroy to these resource
	// addresses (and their dependents).
	TerraformTargets []string

	// RemovePackages uninstalls setup-installed packages (haproxy, httpd, dnsmasq, etc).
	RemovePackages bool

	// KeepISOs skips ISO removal from the Proxmox host; useful when chaining a
	// destroy with an immediate re-deploy.
	KeepISOs bool
}

// NewOptions builds the default destroy Options from cfg and projectRoot.
func NewOptions(cfg *config.Config, projectRoot string) Options {
	return Options{
		BaseOptions: phase.NewBaseOptions(cfg, projectRoot),
		AutoApprove: cfg.Deployment.AutoApprove,
		CleanupKind: cleanup.Full,
	}
}

// Phase drives the destroy flow: terraform destroy, host-file cleanup,
// firewall teardown, and ISO removal. Unlike setup/install/postinstall it
// exposes no StepDefs listing — the authoritative destroy preview is the
// terraform destroy plan (okdctl destroy --dry-run), not a static step list.
type Phase struct {
	phase.BasePhase
}

// New constructs a destroy Phase with the given options.
func New(opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(opts...)
	bp.Log = bp.Log.With("phase", "destroy")
	return &Phase{BasePhase: bp}
}

// Execute tears down the cluster; user confirmation is the CLI layer's
// responsibility before this is called. cfg must be the same cfg passed to
// NewOptions — the two are not re-validated for consistency here.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) ([]distribution.StepResult, error) {
	p.Log.Info("destroy: starting cluster teardown")
	p.Log.Info("destroy: this will permanently remove all vms and generated files")

	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.destroySteps(ctx, cfg, opts))...)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return orchestrator.Results(), err
	}

	return orchestrator.Results(), nil
}
