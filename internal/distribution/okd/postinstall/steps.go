package postinstall

import (
	"context"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// Postinstall StepIDs identify each step in Phase.Run order.
const (
	StepVerifyHealth        distribution.StepID = "verify-health"
	StepCleanupBootstrap    distribution.StepID = "cleanup-bootstrap"
	StepVerifyKubeVIP       distribution.StepID = "verify-kubevip"
	StepDeployProductionDNS distribution.StepID = "deploy-production-dns"
	StepInstallAddons       distribution.StepID = "install-addons"
	StepDisableRHDefaults   distribution.StepID = "disable-rh-defaults"
)

// StepNames maps each postinstall StepID to its display name.
var StepNames = map[distribution.StepID]string{
	StepVerifyHealth:        "verify cluster health",
	StepCleanupBootstrap:    "cleanup bootstrap vm",
	StepVerifyKubeVIP:       "verify kube-vip",
	StepDeployProductionDNS: "deploy production dns",
	StepInstallAddons:       "install addons",
	StepDisableRHDefaults:   "disable rh-subscription-gated defaults",
}

func (p *Phase) postinstallSteps(cfg *config.Config, opts *Options, pctx *distribution.PhaseContext[postInstallContext], mgr *addon.Manager) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepVerifyHealth, Name: StepNames[StepVerifyHealth],
			ReRunSafe:  distribution.ReRunSafeYes,
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
			ID: StepVerifyKubeVIP, Name: StepNames[StepVerifyKubeVIP],
			ReRunSafe:  distribution.ReRunSafeYes,
			NonFatal:   true,
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
		// Verify-before-destroy: the bootstrap VM stays as API fallback until
		// kube-vip is confirmed, unless the operator explicitly skipped verification.
		{
			ID: StepCleanupBootstrap, Name: StepNames[StepCleanupBootstrap],
			ReRunSafe: distribution.ReRunSafeYes,
			SkipWhen: func() bool {
				return !pctx.Get().KubeVIPVerified && !opts.SkipKubeVIP
			},
			SkipReason: "kube-vip not verified — keeping bootstrap vm as fallback",
			Exec: func(ctx context.Context) error {
				if err := p.CleanupBootstrap(ctx, cfg, opts); err != nil {
					return &errtypes.ClusterError{Msg: "bootstrap cleanup failed", Err: err}
				}
				pctx.Update(func(c *postInstallContext) {
					c.BootstrapCleaned = true
				})
				return nil
			},
		},
		{
			ID: StepDeployProductionDNS, Name: StepNames[StepDeployProductionDNS],
			ReRunSafe:  distribution.ReRunSafeYes,
			NonFatal:   true,
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
			ID: StepInstallAddons, Name: StepNames[StepInstallAddons],
			// helm upgrade --install / kubectl apply semantics: re-applying is a safe no-op.
			ReRunSafe: distribution.ReRunSafeYes,
			NonFatal:  true,
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
		{
			ID: StepDisableRHDefaults, Name: StepNames[StepDisableRHDefaults],
			ReRunSafe:  distribution.ReRunSafeYes,
			NonFatal:   true,
			SkipWhen:   func() bool { return opts.KeepRedHatCatalogs },
			SkipReason: "kept by --keep-redhat-catalogs",
			Exec: func(ctx context.Context) error {
				if err := p.disableSubscriptionGatedCatalogSources(ctx); err != nil {
					return &errtypes.ClusterError{Msg: "disable subscription-gated catalogsources", Err: err}
				}
				if err := p.silenceInsightsDisabledAlert(ctx); err != nil {
					return &errtypes.ClusterError{Msg: "silence insights disabled alert", Err: err}
				}
				p.Log.Info("postinstall: disabled rh-subscription-gated catalogsources and insights alert")
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "rh-defaults: disable failed"),
		},
	}
}
