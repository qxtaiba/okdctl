package destroy

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	StepDestroyInfra    distribution.StepID = "destroy-infrastructure"
	StepCleanupFiles    distribution.StepID = "cleanup-files"
	StepCleanupFirewall distribution.StepID = "cleanup-firewall"
	StepPrintSummary    distribution.StepID = "print-summary"
)


func (p *Phase) newDestroyInfraStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepDestroyInfra, "Destroy Infrastructure").
		Description("destroying proxmox infrastructure using terraform").
		Fatal(true).
		SkipWhen(func() bool { return opts.SkipTerraform }).
		SkipReason("Terraform destroy disabled").
		Execute(func(ctx context.Context) error {
			if err := p.destroyInfrastructure(ctx, opts); err != nil {
				return utils.WrapError("infrastructure destruction failed", err)
			}
			p.Log.Info("terraform: infrastructure destruction completed")
			return nil
		}).
		OnError(func(err error) {
			p.Log.Error(fmt.Sprintf("terraform: destruction failed: %v", err))
			if !opts.Force {
				p.Log.Warn("terraform: file cleanup will be skipped unless --force is used")
			}
		}).
		MustBuild()
}


func (p *Phase) newCleanupFilesStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepCleanupFiles, "Cleanup Files").
		Description("performing comprehensive cleanup").
		Fatal(false).
		SkipWhen(func() bool { return opts.SkipCleanup || opts.CleanupType == "" }).
		SkipReason(cleanupFilesSkipReason(opts)).
		Execute(func(ctx context.Context) error {
			cleanupOpts := cleanup.Options{
				Type:           opts.CleanupType,
				WorkDir:        opts.WorkDir,
				ProjectRoot:    opts.ProjectRoot,
				HTTPServerRoot: cfg.HTTPServer.Root,
				HAProxyConfig:  paths.DefaultHAProxyConfigPath,
				VIP:            netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start),
				ClusterName:    cfg.Cluster.Name,
				TerraformEnv:   opts.TerraformEnv,
				PreserveConfig: false,
				RemovePackages: opts.RemovePackages,
				Logger:         p.Log,
			}

			if err := cleanup.Execute(ctx, cleanupOpts); err != nil {
				return utils.WrapError("cleanup failed", err)
			}
			return nil
		}).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("cleanup: file removal failed: %v", err))
		}).
		MustBuild()
}

func cleanupFilesSkipReason(opts Options) string {
	if opts.SkipCleanup {
		return "Cleanup disabled"
	}
	return "No cleanup type specified"
}


func (p *Phase) newCleanupFirewallStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepCleanupFirewall, "Cleanup Firewall").
		Description("removing firewall rules").
		Fatal(false).
		SkipWhen(func() bool { return opts.SkipFirewall }).
		SkipReason("Firewall cleanup disabled").
		Execute(func(ctx context.Context) error {
			if err := system.RemoveOKDFirewallRules(ctx, true, p.Log); err != nil {
				return utils.WrapError("firewall cleanup failed", err)
			}
			p.Log.Info("firewall: okd rules removed from firewalld")
			return nil
		}).
		OnError(func(err error) {
			p.Log.Warn(fmt.Sprintf("firewall: cleanup incomplete: %v", err))
		}).
		MustBuild()
}


func (p *Phase) newPrintSummaryStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepPrintSummary, "Print Summary").
		Description("printing destruction summary").
		Fatal(false).
		OnStart(func() {}).
		Execute(func(ctx context.Context) error {
			p.Log.Info("destroy: cluster teardown completed")
			return nil
		}).
		MustBuild()
}
