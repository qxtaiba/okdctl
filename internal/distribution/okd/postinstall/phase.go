// Package postinstall provides post-install verification and configuration for OKD clusters.
package postinstall

import (
	"context"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// DefaultTimeout is the overall cap applied to post-install verification
// steps that don't override it.
const (
	DefaultTimeout = 10 * time.Minute
)

// Options configures the post-install phase: skip toggles plus per-step
// timeouts for kube-vip verification.
type Options struct {
	phase.BaseOptions
	SkipClusterHealth       bool
	SkipKubeVIP             bool
	Timeout                 time.Duration
	KubeVIPDaemonSetTimeout time.Duration
	KubeVIPVIPTimeout       time.Duration
}

// NewOptions builds post-install Options from cfg and projectRoot, applying
// the default timeout values.
func NewOptions(cfg *config.Config, projectRoot string) Options {
	return Options{
		BaseOptions: phase.BaseOptions{
			ProjectRoot:  projectRoot,
			WorkDir:      filepath.Join(projectRoot, "okd-install"),
			TerraformEnv: phase.GetTerraformEnv(cfg),
		},
		Timeout:                 DefaultTimeout,
		KubeVIPDaemonSetTimeout: DefaultKubeVIPDaemonSetTimeout,
		KubeVIPVIPTimeout:       DefaultKubeVIPVIPTimeout,
	}
}

// Result summarises the observable outcomes of a post-install run.
type Result struct {
	KubeVipIP        string
	BastionIP        string
	NodeCount        int
	BootstrapCleaned bool
	DNSDeployed      bool
}

// Phase drives the post-install flow: cluster-health verification,
// bootstrap cleanup, kube-vip handoff, and production DNS deploy.
type Phase struct {
	phase.BasePhase
}

// New constructs a post-install Phase with the given version tag and options.
func New(version string, opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(version, opts...)
	bp.Log = bp.Log.With("phase", "postinstall")
	return &Phase{BasePhase: bp}
}

// Execute runs the post-install step sequence and returns a summary Result
// along with each step's outcome.
func (p *Phase) Execute(ctx context.Context, cfg *config.Config, opts *Options) (*Result, []distribution.StepResult, error) {
	p.Log.Info("postinstall: starting cluster verification and configuration")

	addonMgr := addon.NewManager(cfg,
		addon.WithExecutor(p.Exec),
		addon.WithLogger(p.Log),
		addon.WithProjectRoot(opts.ProjectRoot),
	)
	pctx := distribution.NewPhaseContext(postInstallContext{})

	orchestrator := distribution.NewOrchestrator(distribution.BuildSteps(p.postinstallSteps(cfg, opts, pctx, addonMgr))...)
	orchestrator.SetLogger(p.Log)
	orchestrator.SetMetricsRecorder(p.Recorder)

	if err := orchestrator.Run(ctx); err != nil {
		return nil, orchestrator.Results(), err
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

	return result, orchestrator.Results(), nil
}

func (p *Phase) deployProductionDNS(ctx context.Context, cfg *config.Config, appsIP, kubeVipIP string, customDomains []templates.DNSCustomDomain) error {
	if err := dns.DeployProduction(ctx, cfg, appsIP, kubeVipIP, customDomains); err != nil {
		return &errtypes.ClusterError{Msg: "failed to deploy production dns config", Err: err}
	}
	return nil
}
