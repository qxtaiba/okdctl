// Package postinstall provides post-install verification and configuration for OKD clusters.
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
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

const (
	DefaultTimeout = 10 * time.Minute
)

type Options struct {
	paths.BaseOptions
	SkipClusterHealth       bool
	SkipKubeVIP             bool
	Timeout                 time.Duration
	KubeVIPDaemonSetTimeout time.Duration
	KubeVIPVIPTimeout       time.Duration
}

func NewOptions(cfg *config.Config, projectRoot string) Options {
	return Options{
		BaseOptions: paths.BaseOptions{
			ProjectRoot:  projectRoot,
			WorkDir:      filepath.Join(projectRoot, "okd-install"),
			TerraformEnv: paths.GetTerraformEnv(cfg),
		},
		Timeout:                 DefaultTimeout,
		KubeVIPDaemonSetTimeout: DefaultKubeVIPDaemonSetTimeout,
		KubeVIPVIPTimeout:       DefaultKubeVIPVIPTimeout,
	}
}

type Result struct {
	KubeVipIP        string
	BastionIP        string
	NodeCount        int
	BootstrapCleaned bool
	DNSDeployed      bool
}

type Phase struct {
	paths.BasePhase
}

func New(exec *executor.Executor, logger utils.Logger, version string) *Phase {
	return &Phase{
		BasePhase: paths.NewBasePhase(exec, logger, version),
	}
}

func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts Options) (*Result, error) {
	p.Log.Info("postinstall: starting cluster verification and configuration")

	addonMgr := addon.NewManager(cfg, p.Exec, p.Log, opts.ProjectRoot)
	pctx := distribution.NewPhaseContext(PostInstallContext{})

	orchestrator := distribution.NewOrchestrator(
		p.NewVerifyHealthStep(cfg, opts, pctx),
		p.NewCleanupBootstrapStep(cfg, opts, pctx),
		p.NewVerifyKubeVIPStep(cfg, opts, pctx),
		p.NewDeployProductionDNSStep(cfg, opts, pctx),
		p.NewInstallAddonsStep(cfg, opts, pctx, addonMgr),
	)
	orchestrator.SetLogger(p.Log)

	if err := orchestrator.Run(ctx); err != nil {
		return nil, err
	}

	state := pctx.Get()
	result := &Result{
		KubeVipIP:        state.KubeVipIP,
		BastionIP:        cfg.Networking.Bastion.IP,
		BootstrapCleaned: state.BootstrapCleaned,
		DNSDeployed:      state.DNSDeployed,
	}
	if state.ClusterHealth != nil {
		result.NodeCount = state.ClusterHealth.ReadyNodes
	}

	p.Log.Info("postinstall: cluster configuration completed successfully")

	return result, nil
}

func (p *Phase) deployProductionDNS(ctx context.Context, cfg *config.Config, appsIP, kubeVipIP string, customDomains []templates.DNSCustomDomain) error {
	if err := dns.DeployProduction(ctx, cfg, appsIP, kubeVipIP, customDomains); err != nil {
		return utils.WrapError("failed to deploy production dns config", err)
	}
	return nil
}
