package install

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

const (
	StepDeployInfra     distribution.StepID = "deploy-infrastructure"
	StepWaitBootstrap   distribution.StepID = "wait-bootstrap"
	StepStartWorkers    distribution.StepID = "start-workers"
	StepSetupKubeconfig distribution.StepID = "setup-kubeconfig"
	StepValidateAccess  distribution.StepID = "validate-access"
	StepMonitorInstall  distribution.StepID = "monitor-install"
	StepSetupAccess     distribution.StepID = "setup-access"
)

// ═══════════════════════════════════════════════════════════════════════════════
// DEPLOY INFRASTRUCTURE STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newDeployInfraStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepDeployInfra, "Deploy Infrastructure").
		Description("deploying proxmox infrastructure using terraform").
		Fatal(true).
		SkipWhen(func() bool { return opts.SkipTerraform }).
		SkipReason("Terraform deployment disabled").
		Execute(func(ctx context.Context) error {
			if err := p.DeployInfrastructure(ctx, cfg, opts); err != nil {
				return utils.WrapError("infrastructure deployment failed", err)
			}
			p.Log.Info("terraform: proxmox infrastructure deployed successfully")
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// WAIT BOOTSTRAP STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newWaitBootstrapStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)
	return distribution.NewStepBuilder(StepWaitBootstrap, "Wait for Bootstrap").
		Description("waiting for bootstrap node to initialize").
		Fatal(true).
		OnStart(func() {
			p.Log.Info("bootstrap: waiting for control plane initialization")
			p.Log.Info("bootstrap: this process typically takes 15-30 minutes")
		}).
		Execute(func(ctx context.Context) error {
			if err := p.WaitForBootstrap(ctx, clusterDir, opts); err != nil {
				return utils.WrapError("bootstrap failed", err)
			}
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// START WORKERS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newStartWorkersStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepStartWorkers, "Start Worker Nodes").
		Description("starting worker nodes after bootstrap complete").
		Fatal(true).
		SkipWhen(func() bool { return opts.SkipTerraform }).
		SkipReason("Terraform deployment disabled").
		Execute(func(ctx context.Context) error {
			return p.StartWorkerVMs(ctx, cfg, opts)
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// SETUP KUBECONFIG STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newSetupKubeconfigStep(opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)
	return distribution.NewStepBuilder(StepSetupKubeconfig, "Setup Kubeconfig").
		Description("configuring cluster access").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			return p.SetupKubeconfig(clusterDir)
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// VALIDATE ACCESS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newValidateAccessStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepValidateAccess, "Validate Cluster Access").
		Description("validating cluster access").
		Fatal(true).
		Execute(func(ctx context.Context) error {
			return p.ValidateClusterAccess(ctx)
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// MONITOR INSTALLATION STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newMonitorInstallStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)
	return distribution.NewStepBuilder(StepMonitorInstall, "Monitor Installation").
		Description("monitoring installation and approving certificate requests").
		Fatal(true).
		OnStart(func() {
			p.Log.Info("install: monitoring cluster operators and approving csrs")
			p.Log.Info("install: this process typically takes 30-60 minutes")
		}).
		Execute(func(ctx context.Context) error {
			if err := p.MonitorInstallation(ctx, clusterDir, opts); err != nil {
				return utils.WrapError("installation monitoring failed", err)
			}
			return nil
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// SETUP CLUSTER ACCESS STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newSetupAccessStep(opts Options) distribution.ProvisioningStep {
	clusterDir := paths.ClusterConfigDir(opts.WorkDir)
	return distribution.NewStepBuilder(StepSetupAccess, "Setup Cluster Access").
		Description("configuring persistent cluster access").
		Fatal(false).
		Execute(func(ctx context.Context) error {
			return p.SetupClusterAccess(ctx, clusterDir)
		}).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("kubeconfig: failed to setup persistent access: %v", err))
		}).
		MustBuild()
}

