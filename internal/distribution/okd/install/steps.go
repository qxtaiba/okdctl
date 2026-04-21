package install

import (
	"context"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// Step IDs for the install phase, ordered as they execute.
const (
	StepDeployInfra     distribution.StepID = "deploy-infrastructure"
	StepWaitBootstrap   distribution.StepID = "wait-bootstrap"
	StepStartWorkers    distribution.StepID = "start-workers"
	StepSetupKubeconfig distribution.StepID = "setup-kubeconfig"
	StepValidateAccess  distribution.StepID = "validate-access"
	StepMonitorInstall  distribution.StepID = "monitor-install"
	StepSetupAccess     distribution.StepID = "setup-access"
)

func (p *Phase) installSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	clusterDir := phase.ClusterConfigDir(opts.WorkDir)

	return []distribution.StepDef{
		{
			ID: StepDeployInfra, Name: "deploy infrastructure",
			Desc:       "deploying proxmox infrastructure using terraform",
			SkipWhen:   func() bool { return opts.SkipTerraform },
			SkipReason: "terraform deployment disabled",
			Exec: func(ctx context.Context) error {
				if err := p.DeployInfrastructure(ctx, cfg, opts); err != nil {
					return &errtypes.ClusterError{Msg: "infrastructure deployment failed", Err: err}
				}
				p.Log.Info("terraform: proxmox infrastructure deployed successfully")
				return nil
			},
		},
		{
			ID: StepWaitBootstrap, Name: "wait for bootstrap",
			Desc: "waiting for bootstrap node to initialize",
			OnStart: func() {
				p.Log.Info("bootstrap: waiting for control plane initialization")
				p.Log.Info("bootstrap: this process typically takes 15-30 minutes")
			},
			Exec: func(ctx context.Context) error {
				if err := p.WaitForBootstrap(ctx, clusterDir, opts); err != nil {
					return &errtypes.ClusterError{Msg: "bootstrap failed", Err: err}
				}
				return nil
			},
		},
		{
			ID: StepStartWorkers, Name: "start worker nodes",
			Desc:       "starting worker nodes after bootstrap complete",
			SkipWhen:   func() bool { return opts.SkipTerraform },
			SkipReason: "terraform deployment disabled",
			Exec:       func(ctx context.Context) error { return p.StartWorkerVMs(ctx, cfg, opts) },
		},
		{
			ID: StepSetupKubeconfig, Name: "setup kubeconfig",
			Desc: "configuring cluster access",
			Exec: func(ctx context.Context) error { return p.SetupKubeconfig(ctx, clusterDir) },
		},
		{
			ID: StepValidateAccess, Name: "validate cluster access",
			Desc: "validating cluster access",
			Exec: func(ctx context.Context) error { return p.ValidateClusterAccess(ctx) },
		},
		{
			ID: StepMonitorInstall, Name: "monitor installation",
			Desc: "monitoring installation and approving certificate requests",
			OnStart: func() {
				p.Log.Info("install: monitoring cluster operators and approving csrs")
				p.Log.Info("install: this process typically takes 30-60 minutes")
			},
			Exec: func(ctx context.Context) error {
				if err := p.MonitorInstallation(ctx, clusterDir, opts, nil); err != nil {
					return &errtypes.ClusterError{Msg: "installation monitoring failed", Err: err}
				}
				return nil
			},
		},
		{
			ID: StepSetupAccess, Name: "setup cluster access",
			Desc: "configuring persistent cluster access", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.SetupClusterAccess(ctx, clusterDir) },
			OnError: phase.WarnOnError(p.Log, "kubeconfig: failed to setup persistent access"),
		},
	}
}
