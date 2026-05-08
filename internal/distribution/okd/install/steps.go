package install

import (
	"context"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
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
			// terraform apply is idempotent: re-running against existing infra
			// is a safe no-op per the Terraform execution model.
			ReRunSafe:  distribution.ReRunSafeYes,
			Desc:       "deploying proxmox infrastructure using terraform",
			SkipWhen:   func() bool { return opts.SkipTerraform },
			SkipReason: "terraform deployment disabled",
			Exec: func(ctx context.Context) error {
				if err := p.DeployInfrastructure(ctx, cfg, opts); err != nil {
					return err
				}
				p.Log.Info("terraform: proxmox infrastructure deployed successfully")
				return nil
			},
		},
		{
			ID: StepWaitBootstrap, Name: "wait for bootstrap",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "waiting for bootstrap node to initialize",
			OnStart: func() {
				p.Log.Info("bootstrap: waiting for control plane initialization")
				p.Log.Info("bootstrap: this process typically takes 15-30 minutes")
			},
			Exec: func(ctx context.Context) error {
				return p.WaitForBootstrap(ctx, clusterDir, opts)
			},
		},
		{
			ID: StepStartWorkers, Name: "start worker nodes",
			ReRunSafe:  distribution.ReRunSafeYes,
			Desc:       "starting worker nodes after bootstrap complete",
			SkipWhen:   func() bool { return opts.SkipTerraform },
			SkipReason: "terraform deployment disabled",
			Exec:       func(ctx context.Context) error { return p.StartWorkerVMs(ctx, cfg, opts) },
		},
		{
			ID: StepSetupKubeconfig, Name: "setup kubeconfig",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "configuring cluster access",
			Exec:      func(ctx context.Context) error { return p.SetupKubeconfig(ctx, clusterDir) },
		},
		{
			ID: StepValidateAccess, Name: "validate cluster access",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "validating cluster access",
			Exec:      func(ctx context.Context) error { return p.ValidateClusterAccess(ctx) },
		},
		{
			ID: StepMonitorInstall, Name: "monitor installation",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "monitoring installation and approving certificate requests",
			OnStart: func() {
				p.Log.Info("install: monitoring cluster operators and approving csrs")
				p.Log.Info("install: this process typically takes 30-60 minutes")
			},
			Exec: func(ctx context.Context) error {
				return p.MonitorInstallation(ctx, clusterDir, opts, nil)
			},
		},
		{
			ID: StepSetupAccess, Name: "setup cluster access",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "configuring persistent cluster access", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.SetupClusterAccess(ctx, clusterDir) },
			OnError: phase.WarnOnError(p.Log, "kubeconfig: failed to setup persistent access"),
		},
	}
}
