package destroy

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
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

// destroyTracker buffers step-level failures/skips for the summary step,
// which must stay NonFatal:false — its joined error is what makes a failed
// teardown exit non-zero.
type destroyTracker struct {
	log      *slog.Logger
	errs     []error
	failures []string
	skipped  []string
}

func (t *destroyTracker) onError(label string) func(error) {
	return func(err error) {
		t.errs = append(t.errs, err)
		t.failures = append(t.failures, label)
		phase.WarnOnError(t.log, label)(err)
	}
}

// skipWhen adapts cause (the fired skip condition, or "" to run) into a
// StepDef SkipWhen/SkipReasonFunc pair, recording "label: reason" for the summary.
func (t *destroyTracker) skipWhen(label string, cause func() string) (when func() bool, reason func() string) {
	var fired string
	when = func() bool {
		fired = cause()
		if fired == "" {
			return false
		}
		t.skipped = append(t.skipped, label+": "+fired)
		return true
	}
	return when, func() string { return fired }
}

// destroySkips holds the resolved SkipWhen/SkipReasonFunc pair per skippable
// destroy step.
type destroySkips struct {
	tf, iso, cleanup, firewall                   func() bool
	tfReason, isoReason, cleanupReason, fwReason func() string
}

func (t *destroyTracker) buildSkips(ctx context.Context, cfg *config.Config, opts *Options, fw *firewall.Firewall) *destroySkips {
	s := &destroySkips{}
	s.tf, s.tfReason = t.skipWhen("terraform", func() string {
		if opts.SkipTerraform {
			return "terraform destroy disabled via --skip-terraform"
		}
		return ""
	})
	s.iso, s.isoReason = t.skipWhen("iso removal", func() string {
		switch {
		case opts.KeepISOs:
			return "iso removal disabled via --keep-isos"
		case cfg.Provider.Proxmox == nil:
			return "no proxmox provider configured"
		case t.terraformFailed():
			return "terraform destroy failed — live vms may still reference these isos"
		}
		return ""
	})
	s.cleanup, s.cleanupReason = t.skipWhen("file cleanup", func() string { return cleanupFilesSkipReason(opts) })
	s.firewall, s.fwReason = t.skipWhen("firewall", func() string {
		switch {
		case opts.SkipFirewall:
			return "firewall cleanup disabled via --skip-firewall"
		case t.terraformFailed():
			return "terraform destroy failed — live vms may still depend on these rules"
		case fw.DetectBackend(ctx) == firewall.None:
			return "no active firewall backend"
		}
		return ""
	})
	return s
}

// labelTerraformDestroy must match track()'s StepDestroyInfra OnError label,
// or terraformFailed() breaks silently.
const labelTerraformDestroy = "terraform destroy"

func (t *destroyTracker) terraformFailed() bool {
	return slices.Contains(t.failures, labelTerraformDestroy)
}

func (p *Phase) destroySteps(ctx context.Context, cfg *config.Config, opts *Options) []distribution.StepDef {
	t := &destroyTracker{log: p.Log}
	track := t.onError
	fw := firewall.New(firewall.WithLogger(p.Log))
	sk := t.buildSkips(ctx, cfg, opts, fw)
	return []distribution.StepDef{
		{
			ID: StepDestroyInfra, Name: "destroy infrastructure", ReRunSafe: distribution.ReRunSafeYes,
			// destroy on already-destroyed infra exits cleanly, so re-runs are safe.
			NonFatal:       true, // orchestrator continues through cleanup steps on TF failure
			SkipWhen:       sk.tf,
			SkipReasonFunc: sk.tfReason,
			Exec: func(ctx context.Context) error {
				if err := p.destroyInfrastructure(ctx, cfg, opts); err != nil {
					return err
				}
				p.Log.Info("terraform: infrastructure destruction completed")
				return nil
			},
			OnError: track(labelTerraformDestroy),
		},
		{
			ID: StepRemoveRemoteISO, Name: "remove remote ISO", ReRunSafe: distribution.ReRunSafeYes,
			NonFatal:       true,
			SkipWhen:       sk.iso,
			SkipReasonFunc: sk.isoReason,
			Exec: func(ctx context.Context) error {
				host := hostssh.ProxmoxBareHost(cfg.Provider.Proxmox.Host)
				knownHostsPath, verifyErr := sshpin.Verify(ctx, host, cfg.Provider.Proxmox.SSHHostFingerprint, cfg.Provider.Proxmox.RequirePinnedFingerprint, p.Log)
				if verifyErr != nil {
					return verifyErr
				}
				params := &hostssh.RemoteISOParams{
					Host:           host,
					Node:           cfg.Provider.Proxmox.Node,
					Exec:           p.Exec,
					Log:            p.Log,
					KnownHostsPath: knownHostsPath,
				}
				if err := hostssh.RemoveFCOSISOFromProxmox(ctx, params, hostssh.DefaultProxmoxISODir); err != nil {
					return err
				}
				return hostssh.RemoveCustomISOsFromProxmox(ctx, params, hostssh.DefaultProxmoxISODir, customISONames(cfg))
			},
			OnError: track("iso removal"),
		},
		{
			ID: StepCleanupFiles, Name: "cleanup files", ReRunSafe: distribution.ReRunSafeYes,
			NonFatal:       true,
			SkipWhen:       sk.cleanup,
			SkipReasonFunc: sk.cleanupReason,
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
				cleanupOpts := cleanup.NewOptions(cfg, opts.ProjectRoot, kind)
				cleanupOpts.VIP = vip
				cleanupOpts.RemovePackages = opts.RemovePackages
				cleanupOpts.PostDestroy = !t.terraformFailed()
				if err := cleanup.New(phase.WithExecutor(p.Exec), phase.WithLogger(p.Log)).Execute(ctx, &cleanupOpts); err != nil {
					return &errtypes.ClusterError{Msg: "cleanup failed", Err: err}
				}
				return nil
			},
			OnError: track("file cleanup"),
		},
		{
			ID: StepCleanupFirewall, Name: "cleanup firewall", ReRunSafe: distribution.ReRunSafeYes,
			NonFatal:       true,
			SkipWhen:       sk.firewall,
			SkipReasonFunc: sk.fwReason,
			Exec: func(ctx context.Context) error {
				if err := fw.RemoveOKDRules(ctx, true); err != nil {
					return &errtypes.ClusterError{Msg: "firewall cleanup failed", Err: err}
				}
				p.Log.Info("firewall: okd rules removed")
				return nil
			},
			OnError: track("firewall cleanup"),
		},
		{
			ID: StepPrintSummary, Name: "print summary", ReRunSafe: distribution.ReRunSafeYes,
			NonFatal: false,
			Exec: func(_ context.Context) error {
				errs, failures, skipped := t.errs, t.failures, t.skipped
				switch {
				case len(failures) > 0:
					args := []any{"failed_steps", failures}
					if len(skipped) > 0 {
						args = append(args, "skipped_steps", skipped)
					}
					p.Log.Warn("destroy: teardown finished with non-fatal failures", args...)
					return &errtypes.ClusterError{Msg: "destroy finished with failed steps", Err: errors.Join(errs...)}
				case len(skipped) > 0:
					p.Log.Info("destroy: cluster teardown completed",
						"skipped_steps", skipped)
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
		return "cleanup disabled via --skip-cleanup"
	}
	if opts.CleanupKind == "" {
		return "no cleanup type specified"
	}
	if !system.DirExists(opts.WorkDir) {
		return "work directory absent"
	}
	return ""
}
