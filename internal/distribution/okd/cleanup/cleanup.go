// Package cleanup removes OKD cluster artifacts best-effort; terraform state
// is removed last so destroy stays re-runnable after a mid-run crash.
package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Kind selects which cleanup steps run.
type Kind string

// Cleanup kinds: Full removes everything; the *Only kinds scope to one subsystem.
const (
	Full          Kind = "full"
	WorkOnly      Kind = "work-only"
	WebOnly       Kind = "web-only"
	HAProxyOnly   Kind = "haproxy-only"
	TerraformOnly Kind = "terraform-only"
)

// ValidKinds returns every recognised cleanup Kind.
func ValidKinds() []Kind {
	return []Kind{Full, WorkOnly, WebOnly, HAProxyOnly, TerraformOnly}
}

// KindStrings returns the string representations of ValidKinds.
func KindStrings() []string {
	ks := ValidKinds()
	ss := make([]string, len(ks))
	for i, k := range ks {
		ss[i] = string(k)
	}
	return ss
}

// IsValid reports whether k is a recognised cleanup Kind.
func (k Kind) IsValid() bool {
	return slices.Contains(ValidKinds(), k)
}

// Validate returns a *errtypes.ConfigError when k is empty or not a recognised Kind.
func (k Kind) Validate() error {
	if k == "" {
		return &errtypes.ConfigError{Msg: "cleanup kind not set"}
	}
	if k.IsValid() {
		return nil
	}
	return &errtypes.ConfigError{
		Msg: fmt.Sprintf("unknown cleanup type: %s (valid: %s)", k, strings.Join(KindStrings(), ", ")),
	}
}

// Options configures a cleanup run.
type Options struct {
	phase.BaseOptions

	Kind           Kind
	HTTPServerRoot string
	HAProxyConfig  string
	VIP            string
	ClusterName    string
	RemovePackages bool
	BinDir         string
	// PostDestroy gates removal of an empty terraform.tfstate after destroy;
	// must not be set on setup-flow runs.
	PostDestroy bool
	// ForceCredentialWipe removes cluster-config even with live terraform
	// state; only set via explicit consent (deploy --fresh).
	ForceCredentialWipe bool
}

// NewOptions builds the default cleanup Options for cfg, projectRoot, and
// kind. VIP, RemovePackages, and PostDestroy must be set by the caller afterward.
func NewOptions(cfg *config.Config, projectRoot string, kind Kind) Options {
	return Options{
		BaseOptions:    phase.NewBaseOptions(cfg, projectRoot),
		Kind:           kind,
		HTTPServerRoot: cfg.HTTPServer.Root,
		HAProxyConfig:  phase.DefaultHAProxyConfigPath,
		ClusterName:    cfg.Cluster.Name,
		BinDir:         config.ResolveBinDir(cfg),
	}
}

// Phase drives a cleanup run; per-step outcomes surface via the summary step's joined error.
type Phase struct {
	phase.BasePhase
}

// New constructs a cleanup Phase with the given options.
func New(opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(opts...)
	bp.Log = bp.Log.With("phase", "cleanup")
	return &Phase{BasePhase: bp}
}

// Execute runs the cleanup steps selected by opts.Kind; step failures
// accumulate into a joined error and a partial run still attempts the rest.
func (p *Phase) Execute(ctx context.Context, opts *Options) error {
	return executeWithRecorder(ctx, opts, p.Log, p.Recorder)
}

// Step IDs for the cleanup phase, ordered as they execute within Full.
const (
	StepCleanupWorkDir       distribution.StepID = "cleanup-workdir"
	StepCleanupWebServer     distribution.StepID = "cleanup-webserver"
	StepCleanupHAProxy       distribution.StepID = "cleanup-haproxy"
	StepCleanupApache        distribution.StepID = "cleanup-apache"
	StepCleanupDnsmasq       distribution.StepID = "cleanup-dnsmasq"
	StepCleanupTerraform     distribution.StepID = "cleanup-terraform"
	StepCleanupPackages      distribution.StepID = "cleanup-packages"
	StepCleanupIgnitionCerts distribution.StepID = "cleanup-ignition-certs"
	StepCleanupSummary       distribution.StepID = "cleanup-summary"
)

// cleanupTracker buffers per-step errors; safe without a mutex since steps run serially.
type cleanupTracker struct {
	errs  []error
	names []string
}

func (t *cleanupTracker) onError(name string) func(error) {
	return func(err error) {
		t.errs = append(t.errs, err)
		t.names = append(t.names, name)
	}
}

// executeWithRecorder runs the cleanup step sequence with optional metrics wiring; rec may be nil.
func executeWithRecorder(ctx context.Context, opts *Options, logger *slog.Logger, rec distribution.MetricsRecorder) error {
	logger = logutil.OrNop(logger)
	if err := opts.Kind.Validate(); err != nil {
		return err
	}
	defs := cleanupSteps(opts, logger)
	o := distribution.NewOrchestrator(distribution.BuildSteps(defs)...)
	o.SetLogger(logger)
	o.SetMetricsRecorder(rec)
	return o.Run(ctx)
}

// retainClusterCredentials preserves cluster-config when terraform state is
// live or corrupt (fail-closed); ForceCredentialWipe bypasses both.
func retainClusterCredentials(opts *Options, logger *slog.Logger) (bool, error) {
	if opts.ForceCredentialWipe {
		return false, nil
	}
	envDir := workspace.TerraformEnvDir(opts.ProjectRoot, opts.TerraformEnv)
	tf := terraform.New(envDir, terraform.WithLogger(logger))
	switch tf.StateStatus() {
	case terraform.StateStatusPopulated:
		logger.Warn("cleanup: terraform state still has resources; preserving cluster credentials (cluster-config with kubeconfig and kubeadmin-password) — run 'okdctl destroy' to tear the cluster down first")
		return true, nil
	case terraform.StateStatusCorrupt:
		return false, &errtypes.ClusterError{
			Msg: fmt.Sprintf("terraform state is corrupt; refusing to remove cluster credentials — restore %s and re-run",
				filepath.Join(envDir, "terraform.tfstate")),
		}
	default:
		return false, nil
	}
}

func cleanupSummaryStep(opts *Options, t *cleanupTracker, logger *slog.Logger) distribution.StepDef {
	return distribution.StepDef{
		ID: StepCleanupSummary, Name: "cleanup summary",
		NonFatal:  false,
		ReRunSafe: distribution.ReRunSafeYes,
		Exec: func(_ context.Context) error {
			printSummary(opts, t, logger)
			return errors.Join(t.errs...)
		},
	}
}

func ignitionCertsCleanupStep(opts *Options, t *cleanupTracker, logger *slog.Logger) distribution.StepDef {
	return distribution.StepDef{
		ID: StepCleanupIgnitionCerts, Name: "cleanup ignition certs",
		NonFatal:  true,
		ReRunSafe: distribution.ReRunSafeYes,
		AlreadyDone: func(_ context.Context) (bool, error) {
			return !system.DirExists(filepath.Join(opts.ProjectRoot, "certs", "ignition")), nil
		},
		Exec:    func(ctx context.Context) error { return IgnitionCerts(ctx, opts.ProjectRoot, logger) },
		OnError: t.onError("ignition-certs"),
	}
}

// terraformCleanupStep removes terraform artifacts and, post-destroy, the state
// file only when provably resource-free.
func terraformCleanupStep(opts *Options, t *cleanupTracker, logger *slog.Logger) distribution.StepDef {
	return distribution.StepDef{
		ID: StepCleanupTerraform, Name: "cleanup terraform",
		NonFatal:    true,
		ReRunSafe:   distribution.ReRunSafeNo,
		AlreadyDone: func(_ context.Context) (bool, error) { return terraformCleanupDone(opts) },
		Exec: func(ctx context.Context) error {
			if err := Terraform(ctx, opts.ProjectRoot, opts.TerraformEnv, logger); err != nil {
				return err
			}
			if !opts.PostDestroy || opts.TerraformEnv == "" {
				return nil
			}
			envDir := workspace.TerraformEnvDir(opts.ProjectRoot, opts.TerraformEnv)
			tf := terraform.New(envDir, terraform.WithLogger(logger))
			switch tf.StateStatus() {
			case terraform.StateStatusEmpty, terraform.StateStatusMissing:
				_ = SafeRemoveWithLogger(ctx, filepath.Join(envDir, "terraform.tfstate"), "terraform state file", logger)
				if kept, pruneErr := tf.PruneBakSnapshotsExceptNewest(); pruneErr != nil {
					logger.Warn("cleanup: terraform state backup prune failed", "err", pruneErr)
				} else if kept != "" {
					logger.Info("cleanup: kept newest terraform state backup as rollback artefact", "path", kept)
				}
			case terraform.StateStatusCorrupt:
				logger.Warn("cleanup: terraform state is corrupt; preserving terraform.tfstate — the VMs it tracked may still be live; restore from a backup and run 'okdctl destroy'",
					"path", filepath.Join(envDir, "terraform.tfstate"), "backup", tf.NewestBakSnapshot())
			}
			return nil
		},
		OnError: t.onError("terraform"),
	}
}

func cleanupSteps(opts *Options, logger *slog.Logger) []distribution.StepDef {
	t := &cleanupTracker{}

	workDirStep := distribution.StepDef{
		ID: StepCleanupWorkDir, Name: "cleanup work directory",
		NonFatal:  true,
		ReRunSafe: distribution.ReRunSafeNo,
		AlreadyDone: func(_ context.Context) (bool, error) {
			return !system.DirExists(opts.WorkDir), nil
		},
		Exec: func(ctx context.Context) error {
			retain, err := retainClusterCredentials(opts, logger)
			if err != nil {
				return err
			}
			return WorkDirectory(ctx, opts.WorkDir, retain, logger)
		},
		OnError: t.onError("work directory"),
	}

	webServerStep := distribution.StepDef{
		ID: StepCleanupWebServer, Name: "cleanup web server",
		NonFatal:  true,
		ReRunSafe: distribution.ReRunSafeYes,
		Exec:      func(ctx context.Context) error { return WebServer(ctx, opts.HTTPServerRoot, logger) },
		OnError:   t.onError("web server"),
	}

	haproxyStep := distribution.StepDef{
		ID: StepCleanupHAProxy, Name: "cleanup haproxy",
		NonFatal:  true,
		ReRunSafe: distribution.ReRunSafeNo,
		AlreadyDone: func(ctx context.Context) (bool, error) {
			return !system.FileExists(opts.HAProxyConfig) && !system.IsServiceActive(ctx, "haproxy"), nil
		},
		Exec:    func(ctx context.Context) error { return HAProxy(ctx, opts.HAProxyConfig, opts.VIP, logger) },
		OnError: t.onError("haproxy"),
	}

	apacheStep := distribution.StepDef{
		ID: StepCleanupApache, Name: "cleanup apache",
		NonFatal:  true,
		ReRunSafe: distribution.ReRunSafeYes,
		Exec:      func(ctx context.Context) error { return Apache(ctx, logger) },
		OnError:   t.onError("apache"),
	}

	dnsmasqStep := distribution.StepDef{
		ID: StepCleanupDnsmasq, Name: "cleanup dnsmasq",
		NonFatal:  true,
		ReRunSafe: distribution.ReRunSafeNo,
		AlreadyDone: func(_ context.Context) (bool, error) {
			if opts.ClusterName == "" {
				return false, nil
			}
			confPath, err := dns.DnsmasqConfigPath(fmt.Sprintf("okd-%s", opts.ClusterName))
			if err != nil {
				return false, &errtypes.ConfigError{Msg: "resolve dnsmasq config path", Err: err}
			}
			return !system.FileExists(confPath), nil
		},
		Exec:    func(ctx context.Context) error { return Dnsmasq(ctx, opts.ClusterName, logger) },
		OnError: t.onError("dnsmasq"),
	}

	terraformStep := terraformCleanupStep(opts, t, logger)

	packagesStep := distribution.StepDef{
		ID: StepCleanupPackages, Name: "cleanup packages",
		NonFatal:   true,
		ReRunSafe:  distribution.ReRunSafeYes,
		SkipWhen:   func() bool { return !opts.RemovePackages },
		SkipReason: "package removal disabled",
		Exec:       func(ctx context.Context) error { return Packages(ctx, opts.BinDir, logger) },
		OnError:    t.onError("packages"),
	}

	ignitionCertsStep := ignitionCertsCleanupStep(opts, t, logger)
	summaryStep := cleanupSummaryStep(opts, t, logger)

	var defs []distribution.StepDef
	switch opts.Kind {
	case Full:
		defs = []distribution.StepDef{workDirStep, webServerStep, haproxyStep, apacheStep, dnsmasqStep, terraformStep, packagesStep, ignitionCertsStep}
	case WorkOnly:
		defs = []distribution.StepDef{workDirStep}
	case WebOnly:
		defs = []distribution.StepDef{webServerStep}
	case HAProxyOnly:
		defs = []distribution.StepDef{haproxyStep}
	case TerraformOnly:
		defs = []distribution.StepDef{terraformStep}
	}
	return append(defs, summaryStep)
}
