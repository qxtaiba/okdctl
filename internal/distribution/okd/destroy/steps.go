package destroy

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/sshpin"
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

// destroyTracker buffers step-level failure and skip labels for the final
// summary step. Without this the prior summary said "cluster teardown
// completed" even after terraform destroy failed — a misleading-success
// regression once StepDestroyInfra became NonFatal.
type destroyTracker struct {
	mu       sync.RWMutex // guards failures and skipped
	log      *slog.Logger
	failures []string
	skipped  []string
}

func (t *destroyTracker) onError(label string) func(error) {
	return func(err error) {
		t.mu.Lock()
		t.failures = append(t.failures, label)
		t.mu.Unlock()
		phase.WarnOnError(t.log, label)(err)
	}
}

func (t *destroyTracker) skipWhen(label string, fn func() bool) func() bool {
	return func() bool {
		if fn() {
			t.mu.Lock()
			t.skipped = append(t.skipped, label)
			t.mu.Unlock()
			return true
		}
		return false
	}
}

func (t *destroyTracker) terraformFailed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return slices.Contains(t.failures, "terraform destroy")
}

func (p *Phase) destroySteps(ctx context.Context, cfg *config.Config, opts *Options) []distribution.StepDef {
	t := &destroyTracker{log: p.Log}
	track, trackSkip := t.onError, t.skipWhen
	fw := firewall.New(firewall.WithLogger(p.Log))
	return []distribution.StepDef{
		{
			ID: StepDestroyInfra, Name: "destroy infrastructure", ReRunSafe: distribution.ReRunSafeYes,
			Desc: "destroying proxmox infrastructure using terraform",
			// terraform destroy on already-destroyed infra exits cleanly (no
			// resources to remove), so re-runs are safe. NonFatal further limits
			// blast radius if the second run encounters a transient TF error.
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
			ID: StepRemoveRemoteISO, Name: "remove remote ISO", ReRunSafe: distribution.ReRunSafeYes,
			Desc:       "removing fedora-coreos iso from proxmox host",
			NonFatal:   true,
			SkipWhen:   trackSkip("iso removal", func() bool { return opts.KeepISOs || cfg.Provider.Proxmox == nil }),
			SkipReason: isoSkipReason(opts, cfg),
			Exec: func(ctx context.Context) error {
				host := phase.ProxmoxBareHost(cfg.Provider.Proxmox.Host)
				knownHostsPath, verifyErr := sshpin.Verify(ctx, host, cfg.Provider.Proxmox.SSHHostFingerprint, p.Log)
				if verifyErr != nil {
					return verifyErr
				}
				params := &phase.RemoteISOParams{
					Host:           host,
					Node:           cfg.Provider.Proxmox.Node,
					Exec:           p.Exec,
					Log:            p.Log,
					KnownHostsPath: knownHostsPath,
				}
				return phase.RemoveFCOSISOFromProxmox(ctx, params, phase.DefaultProxmoxISODir)
			},
			OnError: track("iso removal"),
		},
		{
			ID: StepCleanupFiles, Name: "cleanup files", ReRunSafe: distribution.ReRunSafeYes,
			Desc: "performing comprehensive cleanup", NonFatal: true,
			SkipWhen:   trackSkip("file cleanup", func() bool { return opts.SkipCleanup || opts.CleanupKind == "" || !system.DirExists(opts.WorkDir) }),
			SkipReason: cleanupFilesSkipReason(opts),
			Exec: func(ctx context.Context) error {
				vip, err := phase.ResolveClusterVIP(cfg)
				if err != nil {
					return err
				}
				kind := opts.CleanupKind
				if t.terraformFailed() && kind == cleanup.Full {
					p.Log.Warn("destroy: terraform failed, preserving tfvars / .terraform/ for retry — re-run okdctl destroy to retry")
					kind = cleanup.WorkOnly
				}
				cleanupOpts := &cleanup.Options{
					BaseOptions: phase.BaseOptions{
						WorkDir:      opts.WorkDir,
						ProjectRoot:  opts.ProjectRoot,
						TerraformEnv: opts.TerraformEnv,
					},
					Kind:           kind,
					HTTPServerRoot: cfg.HTTPServer.Root,
					HAProxyConfig:  phase.DefaultHAProxyConfigPath,
					VIP:            vip,
					ClusterName:    cfg.Cluster.Name,
					PreserveConfig: false,
					RemovePackages: opts.RemovePackages,
					BinDir:         phase.ResolveBinDir(cfg),
					PostDestroy:    !t.terraformFailed(),
				}
				if err := cleanup.New(p.Version, phase.WithExecutor(p.Exec), phase.WithLogger(p.Log)).Execute(ctx, cleanupOpts); err != nil {
					return &errtypes.ClusterError{Msg: "cleanup failed", Err: err}
				}
				return nil
			},
			OnError: track("file cleanup"),
		},
		{
			ID: StepCleanupFirewall, Name: "cleanup firewall", ReRunSafe: distribution.ReRunSafeYes,
			Desc: "removing firewall rules", NonFatal: true,
			SkipWhen: trackSkip("firewall", func() bool {
				return opts.SkipFirewall || t.terraformFailed() || fw.DetectBackend(ctx) == firewall.None
			}),
			SkipReason: "firewall cleanup disabled, terraform owns live vms, or no active backend",
			Exec: func(ctx context.Context) error {
				if err := fw.RemoveOKDRules(ctx, true); err != nil {
					return &errtypes.ClusterError{Msg: "firewall cleanup failed", Err: err}
				}
				p.Log.Info("firewall: okd rules removed from firewalld")
				return nil
			},
			OnError: track("firewall cleanup"),
		},
		{
			ID: StepPrintSummary, Name: "print summary", ReRunSafe: distribution.ReRunSafeYes,
			Desc: "printing destruction summary", NonFatal: true,
			Exec: func(_ context.Context) error {
				t.mu.RLock()
				failures := t.failures
				skipped := t.skipped
				t.mu.RUnlock()
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
