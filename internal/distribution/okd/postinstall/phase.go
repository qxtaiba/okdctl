// Package postinstall provides post-install phase types and execution for OKD cluster provisioning.
// It handles cluster health verification, MetalLB configuration, IngressController setup,
// terraform state upload, and DNS configuration.
//
// The Phase struct owns all execution logic and coordinates the verification and configuration
// steps via an orchestrator.
package postinstall

import (
	"context"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/dns"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// Default timeouts for post-install operations.
const (
	DefaultTimeout = 10 * time.Minute
)

// Options configures the post-install phase.
type Options struct {
	paths.BaseOptions

	// SkipClusterHealth skips cluster health verification.
	SkipClusterHealth bool

	// SkipKubeVIP skips kube-vip verification.
	SkipKubeVIP bool

	// Timeout for operations.
	Timeout time.Duration
}

// NewOptions creates Options with defaults.
func NewOptions(cfg *config.Config, projectRoot string) Options {
	return Options{
		BaseOptions: paths.BaseOptions{
			ProjectRoot:  projectRoot,
			WorkDir:      filepath.Join(projectRoot, "okd-install"),
			TerraformEnv: paths.GetTerraformEnv(cfg),
		},
		Timeout: DefaultTimeout,
	}
}

// Result contains information gathered during post-install.
type Result struct {
	RouterLBIP           string
	GrappleberryRouterIP string
	KubeVipIP            string
	NodeCount            int
}

// Phase coordinates the post-install phase execution.
type Phase struct {
	paths.BasePhase
}

// New creates a new post-install phase coordinator.
func New(exec *executor.Executor, logger logging.Logger, version string) *Phase {
	return &Phase{
		BasePhase: paths.NewBasePhase(exec, logger, version),
	}
}

// Execute performs post-installation verification and configuration.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts Options) (*Result, error) {
	p.Log.Info("postinstall: starting cluster verification and configuration")

	addonMgr := addon.NewManager(cfg, p.Exec, p.Log, opts.ProjectRoot)
	pctx := distribution.NewPhaseContext(PostInstallContext{})

	// Order matters: kube-vip verification must come before HAProxy removal,
	// and DNS deployment needs AppsIP from addon outputs.
	orchestrator := distribution.NewOrchestrator(
		p.NewVerifyHealthStep(cfg, opts, pctx),
		p.NewVerifyKubeVIPStep(cfg, opts, pctx),
		p.NewRemoveHAProxyStep(cfg, opts, pctx),
		p.NewInstallAddonsStep(cfg, opts, pctx, addonMgr),
		p.NewDeployProductionDNSStep(cfg, opts, pctx, addonMgr),
	)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return nil, err
	}

	state := pctx.Get()
	result := &Result{
		RouterLBIP: addonMgr.OutputStore().Get("ingress", "router_ip"),
		KubeVipIP:  state.KubeVipIP,
	}
	if state.ClusterHealth != nil {
		result.NodeCount = state.ClusterHealth.ReadyNodes
	}
	result.GrappleberryRouterIP = p.GetGrappleberryRouterIP(ctx)

	p.Log.Info("postinstall: cluster configuration completed successfully")

	return result, nil
}

// deployProductionDNS deploys the production DNS config to dnsmasq.
// With the VIP-from-day-1 architecture, API DNS already points to kube-vip VIP.
// This primarily updates apps.* wildcard and cleans up bootstrap-specific records.
func (p *Phase) deployProductionDNS(ctx context.Context, cfg *config.Config, appsIP, kubeVipIP string) error {
	if err := dns.DeployProduction(ctx, cfg, appsIP, kubeVipIP); err != nil {
		return utils.WrapError("failed to deploy production dns config", err)
	}
	return nil
}
