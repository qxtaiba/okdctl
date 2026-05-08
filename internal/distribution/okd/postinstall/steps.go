package postinstall

import (
	"context"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// Postinstall StepIDs. These identify each step in Phase.Run order
// and appear in distribution.Orchestrator events.
const (
	StepVerifyHealth        distribution.StepID = "verify-health"
	StepCleanupBootstrap    distribution.StepID = "cleanup-bootstrap"
	StepVerifyKubeVIP       distribution.StepID = "verify-kubevip"
	StepDeployProductionDNS distribution.StepID = "deploy-production-dns"
	StepInstallAddons       distribution.StepID = "install-addons"
)

func (p *Phase) postinstallSteps(cfg *config.Config, opts *Options, pctx *distribution.PhaseContext[postInstallContext], mgr *addon.Manager) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepVerifyHealth, Name: "verify cluster health",
			ReRunSafe:  distribution.ReRunSafeYes,
			Desc:       "verifying cluster health",
			SkipWhen:   func() bool { return opts.SkipClusterHealth },
			SkipReason: "cluster health verification skipped by user",
			Exec: func(ctx context.Context) error {
				result, err := p.VerifyClusterHealth(ctx, opts)
				if err != nil {
					return err
				}
				pctx.Update(func(c *postInstallContext) {
					c.ClusterHealth = result
				})
				p.Log.Info("cluster: health check passed", "ready", result.ReadyNodes, "total", result.TotalNodes)
				return nil
			},
		},
		{
			ID: StepCleanupBootstrap, Name: "cleanup bootstrap vm",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "destroying bootstrap vm via terraform",
			NonFatal:  true,
			Exec: func(ctx context.Context) error {
				if err := p.CleanupBootstrap(ctx, cfg, opts); err != nil {
					return &errtypes.ClusterError{Msg: "bootstrap cleanup failed", Err: err}
				}
				pctx.Update(func(c *postInstallContext) {
					c.BootstrapCleaned = true
				})
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "bootstrap: cleanup failed (non-critical)"),
		},
		{
			ID: StepVerifyKubeVIP, Name: "verify kube-vip",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "verifying kube-vip api load balancer", NonFatal: true,
			SkipWhen:   func() bool { return opts.SkipKubeVIP },
			SkipReason: "kube-vip verification skipped by user",
			Exec: func(ctx context.Context) error {
				kubeVipIP, err := p.VerifyKubeVIP(ctx, cfg, opts)
				if err != nil {
					return &errtypes.ClusterError{Msg: "kube-vip verification failed", Err: err}
				}
				pctx.Update(func(c *postInstallContext) {
					c.KubeVIPVerified = true
					c.KubeVipIP = kubeVipIP
				})
				p.Log.Info("kubevip: vip is responding", "vip", kubeVipIP, "port", phase.KubeAPIPort)
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "kubevip: verification failed"),
		},
		{
			ID: StepDeployProductionDNS, Name: "deploy production dns",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "deploying production dns with api vip and apps on bastion", NonFatal: true,
			SkipWhen:   func() bool { return !pctx.Get().KubeVIPVerified },
			SkipReason: "kube-vip not verified, keeping bootstrap dns",
			Exec: func(ctx context.Context) error {
				state := pctx.Get()
				bastionIP := cfg.Networking.Bastion.IP
				if err := p.deployProductionDNS(ctx, cfg, bastionIP, state.KubeVipIP, nil); err != nil {
					return &errtypes.ClusterError{Msg: "production dns deployment failed", Err: err}
				}
				pctx.Update(func(c *postInstallContext) {
					c.DNSDeployed = true
				})
				p.Log.Info("dns: api.* → vip, *.apps → bastion (haproxy)", "vip", state.KubeVipIP, "bastion", bastionIP)
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "dns: production dns deployment failed"),
		},
		{
			ID: StepInstallAddons, Name: "install addons",
			// Addon installation uses helm upgrade --install / kubectl apply
			// semantics; re-applying is a safe no-op for already-installed addons.
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "installing enabled cluster addons", NonFatal: true,
			Exec: func(ctx context.Context) error {
				if err := p.verifyAPIHealthCheck(ctx); err != nil {
					p.Log.Warn("addons: api health check failed before addon install", "err", err)
				}
				if err := mgr.InstallAll(ctx); err != nil {
					return &errtypes.ClusterError{Msg: "addon installation failed", Err: err}
				}
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "addons: installation failed"),
		},
	}
}
