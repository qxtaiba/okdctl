package destroy

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	StepDestroyInfra    distribution.StepID = "destroy-infrastructure"
	StepCleanupFiles    distribution.StepID = "cleanup-files"
	StepCleanupFirewall distribution.StepID = "cleanup-firewall"
	StepPrintSummary    distribution.StepID = "print-summary"
)

// ═══════════════════════════════════════════════════════════════════════════════
// DESTROY INFRASTRUCTURE STEP
// ═══════════════════════════════════════════════════════════════════════════════

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
			p.LogInfo("terraform: infrastructure destruction completed")
			return nil
		}).
		OnError(func(err error) {
			p.LogError(fmt.Sprintf("terraform: destruction failed: %v", err))
			if !opts.Force {
				p.LogWarn("terraform: file cleanup will be skipped unless --force is used")
			}
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// CLEANUP FILES STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newCleanupFilesStep(cfg *config.Config, opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepCleanupFiles, "Cleanup Files").
		Description("performing comprehensive cleanup").
		Fatal(false). // Non-fatal - we want to continue even if cleanup fails
		SkipWhen(func() bool { return opts.SkipCleanup || opts.CleanupType == "" }).
		SkipReason(cleanupFilesSkipReason(opts)).
		Execute(func(ctx context.Context) error {
			cleanupOpts := cleanup.Options{
				Type:           opts.CleanupType,
				WorkDir:        opts.WorkDir,
				ProjectRoot:    opts.ProjectRoot,
				HTTPServerRoot: cfg.HTTPServer.Root,
				HAProxyConfig:  cleanup.DefaultHAProxyConfigPath,
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
			p.LogWarn(fmt.Sprintf("cleanup: file removal failed: %v", err))
		}).
		MustBuild()
}

func cleanupFilesSkipReason(opts Options) string {
	if opts.SkipCleanup {
		return "Cleanup disabled"
	}
	return "No cleanup type specified"
}

// ═══════════════════════════════════════════════════════════════════════════════
// CLEANUP FIREWALL STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newCleanupFirewallStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepCleanupFirewall, "Cleanup Firewall").
		Description("removing firewall rules").
		Fatal(false). // Non-fatal
		SkipWhen(func() bool { return opts.SkipFirewall }).
		SkipReason("Firewall cleanup disabled").
		Execute(func(ctx context.Context) error {
			if err := system.RemoveOKDFirewallRules(ctx, true); err != nil {
				return utils.WrapError("firewall cleanup failed", err)
			}
			p.LogInfo("firewall: okd rules removed from firewalld")
			return nil
		}).
		OnError(func(err error) {
			p.LogWarn(fmt.Sprintf("firewall: cleanup incomplete: %v", err))
		}).
		MustBuild()
}

// ═══════════════════════════════════════════════════════════════════════════════
// PRINT SUMMARY STEP
// ═══════════════════════════════════════════════════════════════════════════════

func (p *Phase) newPrintSummaryStep(opts Options) distribution.ProvisioningStep {
	return distribution.NewStepBuilder(StepPrintSummary, "Print Summary").
		Description("printing destruction summary").
		Fatal(false). // Non-fatal
		OnStart(func() {}).
		Execute(func(ctx context.Context) error {
			p.Log.Info("destroy: cluster teardown completed successfully")
			p.Log.Info("destroy: all vms and generated files have been removed")
			return nil
		}).
		MustBuild()
}
