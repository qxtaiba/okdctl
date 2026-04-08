package destroy

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
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
				return fmt.Errorf("infrastructure destruction failed: %w", err)
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
			vip, err := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)
			if err != nil {
				return fmt.Errorf("failed to derive VIP from static IP start: %w", err)
			}
			cleanupOpts := cleanup.Options{
				Type:           opts.CleanupType,
				WorkDir:        opts.WorkDir,
				ProjectRoot:    opts.ProjectRoot,
				HTTPServerRoot: cfg.HTTPServer.Root,
				HAProxyConfig:  paths.DefaultHAProxyConfigPath,
				VIP:            vip,
				ClusterName:    cfg.Cluster.Name,
				TerraformEnv:   opts.TerraformEnv,
				PreserveConfig: false,
				RemovePackages: opts.RemovePackages,
				Logger:         p.Log,
			}

			if err := cleanup.Execute(ctx, cleanupOpts); err != nil {
				return fmt.Errorf("cleanup failed: %w", err)
			}
			return nil
		}).
		OnError(paths.WarnOnError(p.Log, "cleanup: file removal failed")).
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
			if err := firewall.RemoveOKDRules(ctx, true, p.Log); err != nil {
				return fmt.Errorf("firewall cleanup failed: %w", err)
			}
			p.Log.Info("firewall: okd rules removed from firewalld")
			return nil
		}).
		OnError(paths.WarnOnError(p.Log, "firewall: cleanup incomplete")).
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
