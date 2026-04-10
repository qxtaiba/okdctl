package destroy

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
)

const (
	StepDestroyInfra    distribution.StepID = "destroy-infrastructure"
	StepCleanupFiles    distribution.StepID = "cleanup-files"
	StepCleanupFirewall distribution.StepID = "cleanup-firewall"
	StepPrintSummary    distribution.StepID = "print-summary"
)

func (p *Phase) destroySteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	return []distribution.StepDef{
		{
			ID: StepDestroyInfra, Name: "destroy infrastructure",
			Desc:       "destroying proxmox infrastructure using terraform",
			SkipWhen:   func() bool { return opts.SkipTerraform },
			SkipReason: "terraform destroy disabled",
			Exec: func(ctx context.Context) error {
				if err := p.destroyInfrastructure(ctx, opts); err != nil {
					return fmt.Errorf("infrastructure destruction failed: %w", err)
				}
				p.Log.Info("terraform: infrastructure destruction completed")
				return nil
			},
			OnError: func(err error) {
				p.Log.Error(fmt.Sprintf("terraform: destruction failed: %v", err))
				if !opts.Force {
					p.Log.Warn("terraform: file cleanup will be skipped unless --force is used")
				}
			},
		},
		{
			ID: StepCleanupFiles, Name: "cleanup files",
			Desc: "performing comprehensive cleanup", NonFatal: true,
			SkipWhen:   func() bool { return opts.SkipCleanup || opts.CleanupKind == "" },
			SkipReason: cleanupFilesSkipReason(opts),
			Exec: func(ctx context.Context) error {
				vip, err := netutil.ResolveVIP(cfg.Networking.Bastion.VIP, cfg.Networking.StaticIP.Start)
				if err != nil {
					return fmt.Errorf("failed to resolve VIP: %w", err)
				}
				cleanupOpts := &cleanup.Options{
					Kind:           opts.CleanupKind,
					WorkDir:        opts.WorkDir,
					ProjectRoot:    opts.ProjectRoot,
					HTTPServerRoot: cfg.HTTPServer.Root,
					HAProxyConfig:  phase.DefaultHAProxyConfigPath,
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
			},
			OnError: phase.WarnOnError(p.Log, "cleanup: file removal failed"),
		},
		{
			ID: StepCleanupFirewall, Name: "cleanup firewall",
			Desc: "removing firewall rules", NonFatal: true,
			SkipWhen:   func() bool { return opts.SkipFirewall },
			SkipReason: "firewall cleanup disabled",
			Exec: func(ctx context.Context) error {
				if err := firewall.RemoveOKDRules(ctx, true, p.Log); err != nil {
					return fmt.Errorf("firewall cleanup failed: %w", err)
				}
				p.Log.Info("firewall: okd rules removed from firewalld")
				return nil
			},
			OnError: phase.WarnOnError(p.Log, "firewall: cleanup incomplete"),
		},
		{
			ID: StepPrintSummary, Name: "print summary",
			Desc: "printing destruction summary", NonFatal: true,
			Exec: func(_ context.Context) error {
				p.Log.Info("destroy: cluster teardown completed")
				return nil
			},
		},
	}
}

func cleanupFilesSkipReason(opts *Options) string {
	if opts.SkipCleanup {
		return "cleanup disabled"
	}
	return "no cleanup type specified"
}
