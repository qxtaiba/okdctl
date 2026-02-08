package postinstall

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/addon"
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

const (
	StepVerifyHealth       distribution.StepID = "verify-health"
	StepCleanupBootstrap   distribution.StepID = "cleanup-bootstrap"
	StepVerifyKubeVIP      distribution.StepID = "verify-kubevip"
	StepDeployProductionDNS distribution.StepID = "deploy-production-dns"
	StepInstallAddons      distribution.StepID = "install-addons"
)

func (p *Phase) NewVerifyHealthStep(cfg *config.Config, opts Options, pctx *distribution.PhaseContext[PostInstallContext]) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepVerifyHealth, "Verify Cluster Health").
		Description("verifying cluster health").
		Fatal(true).
		SkipWhen(func() bool {
			return opts.SkipClusterHealth
		}).
		SkipReason("cluster health verification skipped by user").
		Execute(func(ctx context.Context) error {
			result, err := p.VerifyClusterHealth(ctx, opts)
			if err != nil {
				return utils.WrapError("cluster health verification failed", err)
			}
			pctx.Update(func(c *PostInstallContext) {
				c.ClusterHealth = result
			})
			p.Log.Info(fmt.Sprintf("cluster: health check passed (%d/%d nodes ready)", result.ReadyNodes, result.TotalNodes))
			return nil
		}).
		MustBuild()
}

func (p *Phase) NewCleanupBootstrapStep(cfg *config.Config, opts Options, pctx *distribution.PhaseContext[PostInstallContext]) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepCleanupBootstrap, "Cleanup Bootstrap VM").
		Description("destroying bootstrap vm via terraform").
		Fatal(false).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("bootstrap: cleanup failed (non-critical): %v", err))
		}).
		Execute(func(ctx context.Context) error {
			if err := p.CleanupBootstrap(ctx, cfg, opts); err != nil {
				return utils.WrapError("bootstrap cleanup failed", err)
			}
			pctx.Update(func(c *PostInstallContext) {
				c.BootstrapCleaned = true
			})
			return nil
		}).
		MustBuild()
}

func (p *Phase) NewVerifyKubeVIPStep(cfg *config.Config, opts Options, pctx *distribution.PhaseContext[PostInstallContext]) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepVerifyKubeVIP, "Verify kube-vip").
		Description("verifying kube-vip api load balancer").
		Fatal(false).
		SkipWhen(func() bool {
			return opts.SkipKubeVIP
		}).
		SkipReason("kube-vip verification skipped by user").
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("kubevip: verification failed: %v", err))
		}).
		Execute(func(ctx context.Context) error {
			kubeVipIP, err := p.VerifyKubeVIP(ctx, cfg, opts)
			if err != nil {
				return utils.WrapError("kube-vip verification failed", err)
			}
			pctx.Update(func(c *PostInstallContext) {
				c.KubeVIPVerified = true
				c.KubeVipIP = kubeVipIP
			})
			p.Log.Info(fmt.Sprintf("kubevip: vip %s is responding on port 6443", kubeVipIP))
			return nil
		}).
		MustBuild()
}

func (p *Phase) NewDeployProductionDNSStep(cfg *config.Config, opts Options, pctx *distribution.PhaseContext[PostInstallContext]) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepDeployProductionDNS, "Deploy Production DNS").
		Description("deploying production dns with api vip and apps on bastion").
		Fatal(false).
		SkipWhen(func() bool {
			return !pctx.Get().KubeVIPVerified
		}).
		SkipReason("kube-vip not verified — keeping bootstrap dns").
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("dns: production dns deployment failed: %v", err))
		}).
		Execute(func(ctx context.Context) error {
			state := pctx.Get()
			bastionIP := cfg.Networking.Bastion.IP
			if err := p.deployProductionDNS(ctx, cfg, bastionIP, state.KubeVipIP, nil); err != nil {
				return utils.WrapError("production dns deployment failed", err)
			}
			pctx.Update(func(c *PostInstallContext) {
				c.DNSDeployed = true
			})
			p.Log.Info(fmt.Sprintf("dns: api.* → vip %s, *.apps → bastion %s (haproxy)", state.KubeVipIP, bastionIP))
			return nil
		}).
		MustBuild()
}

func (p *Phase) NewInstallAddonsStep(cfg *config.Config, opts Options, pctx *distribution.PhaseContext[PostInstallContext], mgr *addon.Manager) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepInstallAddons, "Install Addons").
		Description("installing enabled cluster addons").
		Fatal(false).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("addons: installation failed: %v", err))
		}).
		Execute(func(ctx context.Context) error {
			if err := p.verifyAPIHealthCheck(ctx); err != nil {
				p.Log.Warn(fmt.Sprintf("addons: api health check failed before addon install: %v", err))
			}
			if err := mgr.InstallAll(ctx); err != nil {
				return utils.WrapError("addon installation failed", err)
			}
			return nil
		}).
		MustBuild()
}
