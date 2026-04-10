package postinstall

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
)

const (
	StepVerifyHealth        distribution.StepID = "verify-health"
	StepCleanupBootstrap    distribution.StepID = "cleanup-bootstrap"
	StepVerifyKubeVIP       distribution.StepID = "verify-kubevip"
	StepDeployProductionDNS distribution.StepID = "deploy-production-dns"
	StepInstallAddons       distribution.StepID = "install-addons"
)

func (p *Phase) postinstallSteps(cfg *config.Config, opts *Options, pctx *distribution.PhaseContext[PostInstallContext], mgr *addon.Manager) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepVerifyHealth, Name: "verify cluster health",
			Desc:       "verifying cluster health",
			SkipWhen:   func() bool { return opts.SkipClusterHealth },
			SkipReason: "cluster health verification skipped by user",
			Exec: func(ctx context.Context) error {
				result, err := p.VerifyClusterHealth(ctx, opts)
				if err != nil {
					return fmt.Errorf("cluster health verification failed: %w", err)
				}
				pctx.Update(func(c *PostInstallContext) {
					c.ClusterHealth = result
				})
				p.Log.Info(fmt.Sprintf("cluster: health check passed (%d/%d nodes ready)", result.ReadyNodes, result.TotalNodes))
				return nil
			},
		},
		{
			ID: StepCleanupBootstrap, Name: "cleanup bootstrap vm",
			Desc: "destroying bootstrap vm via terraform", NonFatal: true,
			Exec: func(ctx context.Context) error {
				if err := p.CleanupBootstrap(ctx, cfg, opts); err != nil {
					return fmt.Errorf("bootstrap cleanup failed: %w", err)
				}
				pctx.Update(func(c *PostInstallContext) {
					c.BootstrapCleaned = true
				})
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "bootstrap: cleanup failed (non-critical)"),
		},
		{
			ID: StepVerifyKubeVIP, Name: "verify kube-vip",
			Desc: "verifying kube-vip api load balancer", NonFatal: true,
			SkipWhen:   func() bool { return opts.SkipKubeVIP },
			SkipReason: "kube-vip verification skipped by user",
			Exec: func(ctx context.Context) error {
				kubeVipIP, err := p.VerifyKubeVIP(ctx, cfg, opts)
				if err != nil {
					return fmt.Errorf("kube-vip verification failed: %w", err)
				}
				pctx.Update(func(c *PostInstallContext) {
					c.KubeVIPVerified = true
					c.KubeVipIP = kubeVipIP
				})
				p.Log.Info(fmt.Sprintf("kubevip: vip %s is responding on port 6443", kubeVipIP))
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "kubevip: verification failed"),
		},
		{
			ID: StepDeployProductionDNS, Name: "deploy production dns",
			Desc: "deploying production dns with api vip and apps on bastion", NonFatal: true,
			SkipWhen:   func() bool { return !pctx.Get().KubeVIPVerified },
			SkipReason: "kube-vip not verified, keeping bootstrap dns",
			Exec: func(ctx context.Context) error {
				state := pctx.Get()
				bastionIP := cfg.Networking.Bastion.IP
				if err := p.deployProductionDNS(ctx, cfg, bastionIP, state.KubeVipIP, nil); err != nil {
					return fmt.Errorf("production dns deployment failed: %w", err)
				}
				pctx.Update(func(c *PostInstallContext) {
					c.DNSDeployed = true
				})
				p.Log.Info(fmt.Sprintf("dns: api.* → vip %s, *.apps → bastion %s (haproxy)", state.KubeVipIP, bastionIP))
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "dns: production dns deployment failed"),
		},
		{
			ID: StepInstallAddons, Name: "install addons",
			Desc: "installing enabled cluster addons", NonFatal: true,
			Exec: func(ctx context.Context) error {
				if err := p.verifyAPIHealthCheck(ctx); err != nil {
					p.Log.Warn(fmt.Sprintf("addons: api health check failed before addon install: %v", err))
				}
				if err := mgr.InstallAll(ctx); err != nil {
					return fmt.Errorf("addon installation failed: %w", err)
				}
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "addons: installation failed"),
		},
	}
}
