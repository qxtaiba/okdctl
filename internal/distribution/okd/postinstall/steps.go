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
	StepVerifyHealth        distribution.StepID = "verify-health"
	StepVerifyKubeVIP       distribution.StepID = "verify-kubevip"
	StepRemoveHAProxy       distribution.StepID = "remove-haproxy"
	StepInstallAddons       distribution.StepID = "install-addons"
	StepDeployProductionDNS distribution.StepID = "deploy-production-dns"
)

// ═══════════════════════════════════════════════════════════════════════════════
// VERIFY HEALTH STEP
// ═══════════════════════════════════════════════════════════════════════════════

// NewVerifyHealthStep creates a step that verifies cluster health after installation.
// Writes ClusterHealth to the phase context for result building.
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

// ═══════════════════════════════════════════════════════════════════════════════
// VERIFY KUBE-VIP STEP
// ═══════════════════════════════════════════════════════════════════════════════

// NewVerifyKubeVIPStep creates a step that verifies kube-vip is functioning correctly.
// Writes KubeVIPVerified and APIIP to the phase context for HAProxy removal and DNS configuration.
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

// ═══════════════════════════════════════════════════════════════════════════════
// REMOVE HAPROXY STEP
// ═══════════════════════════════════════════════════════════════════════════════

// NewRemoveHAProxyStep creates a step that removes HAProxy from the bastion after kube-vip is verified.
// Only runs if kube-vip verification succeeded (KubeVIPVerified is true in context).
func (p *Phase) NewRemoveHAProxyStep(cfg *config.Config, opts Options, pctx *distribution.PhaseContext[PostInstallContext]) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepRemoveHAProxy, "Remove HAProxy").
		Description("removing haproxy from bastion").
		Fatal(false).
		SkipWhen(func() bool {
			// Skip if kube-vip wasn't verified - HAProxy is still needed
			return !pctx.Get().KubeVIPVerified
		}).
		SkipReason("kube-vip not verified - keeping HAProxy as fallback").
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("haproxy: removal failed: %v", err))
		}).
		Execute(func(ctx context.Context) error {
			vip := pctx.Get().KubeVipIP
			if err := p.RemoveHAProxy(ctx, vip); err != nil {
				return utils.WrapError("haproxy removal failed", err)
			}
			p.Log.Info("haproxy: service stopped and disabled on bastion")
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// INSTALL ADDONS STEP
// ═══════════════════════════════════════════════════════════════════════════════

// NewInstallAddonsStep creates a step that installs all enabled addons via the addon manager.
// Addons like MetalLB and Ingress (previously hardcoded steps) are now handled here.
// The addon manager resolves dependencies and installs in the correct order.
func (p *Phase) NewInstallAddonsStep(cfg *config.Config, opts Options, pctx *distribution.PhaseContext[PostInstallContext], mgr *addon.Manager) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepInstallAddons, "Install Addons").
		Description("installing enabled cluster addons").
		Fatal(false).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("addons: installation failed: %v", err))
		}).
		Execute(func(ctx context.Context) error {
			if err := mgr.InstallAll(ctx); err != nil {
				return utils.WrapError("addon installation failed", err)
			}
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// DEPLOY PRODUCTION DNS STEP
// ═══════════════════════════════════════════════════════════════════════════════

// NewDeployProductionDNSStep creates a step that deploys production DNS configuration.
// Since API DNS already points to kube-vip VIP from day 1, this step primarily updates
// the apps wildcard DNS to point to the MetalLB-assigned ingress IP.
// Reads AppsIP from addon outputs (ingress addon's "router_ip" output).
func (p *Phase) NewDeployProductionDNSStep(cfg *config.Config, opts Options, pctx *distribution.PhaseContext[PostInstallContext], mgr *addon.Manager) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepDeployProductionDNS, "Update Apps DNS").
		Description("updating apps wildcard dns for ingress load balancer").
		Fatal(false).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("dns: apps dns update failed: %v", err))
		}).
		Execute(func(ctx context.Context) error {
			state := pctx.Get()
			appsIP := mgr.OutputStore().Get("ingress", "router_ip")
			if err := p.deployProductionDNS(ctx, cfg, appsIP, state.KubeVipIP); err != nil {
				return utils.WrapError("apps dns update failed", err)
			}
			if appsIP != "" {
				p.Log.Info(fmt.Sprintf("dns: apps.* wildcard now points to %s", appsIP))
			} else {
				p.Log.Info("dns: production config deployed (apps ip pending metallb assignment)")
			}
			return nil
		}).
		MustBuild()
}

