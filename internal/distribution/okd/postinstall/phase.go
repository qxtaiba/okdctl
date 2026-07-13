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
	KeepRedHatCatalogs      bool
	Timeout                 time.Duration
	KubeVIPDaemonSetTimeout time.Duration
	KubeVIPVIPTimeout       time.Duration
}

// NewOptions builds post-install Options from cfg and projectRoot, applying
// the default timeout values. It returns a value so each caller receives an
// independent, mutable copy of the defaults; callers pass &opts to Execute
// once they have finished configuring the option set.
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

// New constructs a post-install Phase with the given options.
func New(opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(opts...)
	bp.Log = bp.Log.With("phase", "postinstall")
	return &Phase{BasePhase: bp}
}

// Execute runs the post-install step sequence and returns a summary Result
// along with each step's outcome. cfg must be the same cfg passed to
// NewOptions — opts was derived from it and the two are not re-validated
// for consistency here.
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

	p.Log.Info("postinstall: cluster configuration completed")

	return result, orchestrator.Results(), nil
}

// deployProductionDNSFn and deployBootstrapDNSFn are package-level vars so
// tests can exercise update-ingress flows without mutating /etc/dnsmasq.d,
// mirroring the fn-var seams in internal/distribution/okd/dns.
var (
	deployProductionDNSFn = dns.DeployProduction
	deployBootstrapDNSFn  = dns.DeployBootstrap
)

// StepDefs returns the ordered step definitions this phase executes for
// cfg/opts, without running them. Provisioner.DeploySteps calls this for
// the deploy --dry-run listing; the addon.Manager and PhaseContext it builds
// are throwaway since nothing here invokes Exec.
func (p *Phase) StepDefs(cfg *config.Config, opts *Options) []distribution.StepDef {
	addonMgr := addon.NewManager(cfg,
		addon.WithExecutor(p.Exec),
		addon.WithLogger(p.Log),
		addon.WithProjectRoot(opts.ProjectRoot),
	)
	pctx := distribution.NewPhaseContext(postInstallContext{})
	return p.postinstallSteps(cfg, opts, pctx, addonMgr)
}

func (p *Phase) deployProductionDNS(ctx context.Context, cfg *config.Config, appsIP, kubeVipIP string, customDomains []templates.DNSCustomDomain) error {
	if err := deployProductionDNSFn(ctx, cfg, appsIP, kubeVipIP, customDomains); err != nil {
		return &errtypes.ClusterError{Msg: "failed to deploy production dns config", Err: err}
	}
	return nil
}
