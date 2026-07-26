package install

import (
	"context"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/workspace"
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

// StepNames maps each install StepID to its display name. StepDef literals in
// this file reference this map so each name has a single source.
var StepNames = map[distribution.StepID]string{
	StepDeployInfra:     "deploy infrastructure",
	StepWaitBootstrap:   "wait for bootstrap",
	StepStartWorkers:    "start worker nodes",
	StepSetupKubeconfig: "setup kubeconfig",
	StepValidateAccess:  "validate cluster access",
	StepMonitorInstall:  "monitor installation",
	StepSetupAccess:     "setup cluster access",
}

func (p *Phase) installSteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	clusterDir := workspace.ClusterConfigDir(opts.WorkDir)

	return []distribution.StepDef{
		{
			ID: StepDeployInfra, Name: StepNames[StepDeployInfra],
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
				p.Log.Info("terraform: proxmox infrastructure deployed")
				return nil
			},
		},
		{
			ID: StepWaitBootstrap, Name: StepNames[StepWaitBootstrap],
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
			ID: StepStartWorkers, Name: StepNames[StepStartWorkers],
			ReRunSafe:   distribution.ReRunSafeYes,
			Desc:        "starting worker nodes after bootstrap complete",
			AlreadyDone: func(ctx context.Context) (bool, error) { return p.workersAlreadyRunning(ctx, cfg) },
			SkipWhen:    func() bool { return opts.SkipTerraform },
			SkipReason:  "terraform deployment disabled",
			Exec:        func(ctx context.Context) error { return p.StartWorkerVMs(ctx, cfg, opts) },
		},
		{
			ID: StepSetupKubeconfig, Name: StepNames[StepSetupKubeconfig],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "configuring cluster access",
			Exec:      func(ctx context.Context) error { return p.SetupKubeconfig(ctx, clusterDir) },
		},
		{
			ID: StepValidateAccess, Name: StepNames[StepValidateAccess],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "validating cluster access",
			Exec:      func(ctx context.Context) error { return p.ValidateClusterAccess(ctx) },
		},
		{
			ID: StepMonitorInstall, Name: StepNames[StepMonitorInstall],
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
			ID: StepSetupAccess, Name: StepNames[StepSetupAccess],
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:      "configuring persistent cluster access", NonFatal: true,
			Exec:    func(ctx context.Context) error { return p.SetupClusterAccess(ctx, clusterDir) },
			OnError: phase.WarnOnError(p.Log, "kubeconfig: failed to setup persistent access"),
		},
	}
}
