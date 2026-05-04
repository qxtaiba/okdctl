package destroy

import (
	"context"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Step IDs for the destroy phase, ordered as they execute.
const (
	StepDestroyInfra    distribution.StepID = "destroy-infrastructure"
	StepRemoveRemoteISO distribution.StepID = "remove-remote-iso"
	StepCleanupFiles    distribution.StepID = "cleanup-files"
	StepCleanupFirewall distribution.StepID = "cleanup-firewall"
	StepPrintSummary    distribution.StepID = "print-summary"
)

func (p *Phase) destroySteps(cfg *config.Config, opts *Options) []distribution.StepDef {
	// failures lets the final summary step report accurate state when an
	// earlier NonFatal step errored. Without this the prior summary said
	// "cluster teardown completed" even after terraform destroy failed —
	// a misleading-success regression after StepDestroyInfra became NonFatal.
	// Safe without a mutex because Orchestrator.Run iterates steps serially;
	// add a sync.Mutex if step parallelism ever lands.
	var failures []string
	var skipped []string
	track := func(label string) func(err error) {
		return func(err error) {
			failures = append(failures, label)
			phase.WarnOnError(p.Log, label)(err)
		}
	}
	// trackSkip wraps a SkipWhen predicate to record the step label when skipped.
	// Safe without a mutex: same serial-execution invariant as failures.
	trackSkip := func(label string, fn func() bool) func() bool {
		return func() bool {
			if fn() {
				skipped = append(skipped, label)
				return true
			}
			return false
		}
	}
	return []distribution.StepDef{
		{
			ID: StepDestroyInfra, Name: "destroy infrastructure",
			ReRunSafe: distribution.ReRunSafeNo,
			Desc:       "destroying proxmox infrastructure using terraform",
			NonFatal:   true, // orchestrator continues through cleanup steps on TF failure
			SkipWhen:   trackSkip("terraform", func() bool { return opts.SkipTerraform }),
			SkipReason: "terraform destroy disabled",
			Exec: func(ctx context.Context) error {
				if err := p.destroyInfrastructure(ctx, opts); err != nil {
					return err
				}
				p.Log.Info("terraform: infrastructure destruction completed")
				return nil
			},
			OnError: track("terraform destroy"),
		},
		{
			ID: StepRemoveRemoteISO, Name: "remove remote ISO",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc:       "removing fedora-coreos iso from proxmox host",
			NonFatal:   true,
			SkipWhen:   trackSkip("iso removal", func() bool { return opts.KeepISOs || cfg.Provider.Proxmox == nil }),
			SkipReason: isoSkipReason(opts, cfg),
			Exec: func(ctx context.Context) error {
				params := &phase.RemoteISOParams{
					Host: phase.ProxmoxBareHost(cfg.Provider.Proxmox.Host),
					Node: cfg.Provider.Proxmox.Node,
					Exec: p.Exec,
					Log:  p.Log,
				}
				return phase.RemoveFCOSISOFromProxmox(ctx, params, phase.DefaultProxmoxISODir)
			},
			OnError: track("iso removal"),
		},
		{
			ID: StepCleanupFiles, Name: "cleanup files",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc: "performing comprehensive cleanup", NonFatal: true,
			SkipWhen:   trackSkip("file cleanup", func() bool { return opts.SkipCleanup || opts.CleanupKind == "" || !system.DirExists(opts.WorkDir) }),
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
					BinDir:         phase.ResolveBinDir(cfg),
					Logger:         p.Log,
				}
				if err := cleanup.New(p.Exec, p.Log, p.Version).Execute(ctx, cleanupOpts); err != nil {
					return &errtypes.ClusterError{Msg: "cleanup failed", Err: err}
				}
				return nil
			},
			OnError: track("file cleanup"),
		},
		{
			ID: StepCleanupFirewall, Name: "cleanup firewall",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc: "removing firewall rules", NonFatal: true,
			// context.Background() is safe: DetectBackend runs only exec.LookPath + a bounded systemctl probe.
			SkipWhen: trackSkip("firewall", func() bool {
				return opts.SkipFirewall || firewall.DetectBackend(context.Background(), p.Log) == firewall.None
			}),
			SkipReason: "firewall cleanup disabled or no active backend",
			Exec: func(ctx context.Context) error {
				if err := firewall.RemoveOKDRules(ctx, true, p.Log); err != nil {
					return &errtypes.ClusterError{Msg: "firewall cleanup failed", Err: err}
				}
				p.Log.Info("firewall: okd rules removed from firewalld")
				return nil
			},
			OnError: track("firewall cleanup"),
		},
		{
			ID: StepPrintSummary, Name: "print summary",
			ReRunSafe: distribution.ReRunSafeYes,
			Desc: "printing destruction summary", NonFatal: true,
			Exec: func(_ context.Context) error {
				switch {
				case len(failures) > 0:
					p.Log.Warn("destroy: teardown finished with non-fatal failures",
						"steps", strings.Join(failures, ", "))
				case len(skipped) > 0:
					p.Log.Info("destroy: cluster teardown completed",
						"skipped", strings.Join(skipped, ", "))
				default:
					p.Log.Info("destroy: cluster teardown completed")
				}
				return nil
			},
		},
	}
}

func cleanupFilesSkipReason(opts *Options) string {
	if opts.SkipCleanup {
		return "cleanup disabled"
	}
	if opts.CleanupKind == "" {
		return "no cleanup type specified"
	}
	if !system.DirExists(opts.WorkDir) {
		return "work directory absent"
	}
	return ""
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
