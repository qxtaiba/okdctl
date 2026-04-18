package destroy

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
)

const (
	StepDestroyInfra    distribution.StepID = "destroy-infrastructure"
	StepRemoveRemoteISO distribution.StepID = "remove-remote-iso"
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
			ID: StepRemoveRemoteISO, Name: "remove remote ISO",
			Desc:       "removing fedora-coreos iso from proxmox host",
			NonFatal:   true,
			SkipWhen:   func() bool { return opts.KeepISOs || cfg.Provider.Proxmox == nil },
			SkipReason: isoSkipReason(opts, cfg),
			Exec: func(ctx context.Context) error {
				params := &phase.RemoteISOParams{
					Host: proxmoxBareHost(cfg.Provider.Proxmox.Host),
					Node: cfg.Provider.Proxmox.Node,
					Exec: p.Exec,
					Log:  p.Log,
				}
				return phase.RemoveFCOSISOFromProxmox(ctx, params, setup.DefaultProxmoxISODir)
			},
			OnError: phase.WarnOnError(p.Log, "iso: remote removal incomplete"),
		},
		{
			ID: StepCleanupFiles, Name: "cleanup files",
			Desc: "performing comprehensive cleanup", NonFatal: true,
			SkipWhen:   func() bool { return opts.SkipCleanup || opts.CleanupKind == "" },
			SkipReason: cleanupFilesSkipReason(opts),
			Exec: func(ctx context.Context) error {
				vip, err := phase.ResolveClusterVIP(cfg)
				if err != nil {
					return err
				}
				cleanupOpts := &cleanup.Options{
					BaseOptions: phase.BaseOptions{
						WorkDir:      opts.WorkDir,
						ProjectRoot:  opts.ProjectRoot,
						TerraformEnv: opts.TerraformEnv,
					},
					Kind:           opts.CleanupKind,
					HTTPServerRoot: cfg.HTTPServer.Root,
					HAProxyConfig:  phase.DefaultHAProxyConfigPath,
					VIP:            vip,
					ClusterName:    cfg.Cluster.Name,
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

func isoSkipReason(opts *Options, cfg *config.Config) string {
	if opts.KeepISOs {
		return "iso removal skipped via --keep-isos"
	}
	if cfg.Provider.Proxmox == nil {
		return "no proxmox provider configured"
	}
	return ""
}

// proxmoxBareHost strips any port suffix from the host so it can be passed to
// ssh. Proxmox hosts in config may appear as "host:8006".
func proxmoxBareHost(host string) string {
	if strings.Contains(host, ":") {
		h, _, err := net.SplitHostPort(host)
		if err == nil {
			return h
		}
	}
	// Strip scheme if present (e.g. "https://host")
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	return host
}
