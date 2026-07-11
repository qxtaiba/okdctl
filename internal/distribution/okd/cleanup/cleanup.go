// Package cleanup provides utilities for removing OKD cluster artifacts.
// Cleanup is best-effort: a mid-run crash leaves workDir in a partially-removed
// state with no resume capability. Terraform state is removed last so destroy
// stays re-runnable as long as earlier steps have not corrupted it.
package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Kind selects which cleanup steps run.
type Kind string

// Cleanup kinds. Full removes everything; the *Only kinds scope cleanup to
// a single subsystem.
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

// KindStrings returns the string representations of ValidKinds, suitable for
// error messages and help text.
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

// Validate returns a *errtypes.ConfigError when k is not a recognised Kind.
func (k Kind) Validate() error {
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
	PreserveConfig bool
	HTTPServerRoot string
	HAProxyConfig  string
	VIP            string
	ClusterName    string
	RemovePackages bool
	BinDir         string
	// PostDestroy gates removal of an empty terraform.tfstate after a
	// successful terraform destroy. Must not be set on prepare-flow runs.
	PostDestroy bool
}

// Phase drives a cleanup run.
type Phase struct {
	phase.BasePhase
}

// New constructs a cleanup Phase with the given version tag and options.
func New(version string, opts ...phase.BasePhaseOption) *Phase {
	bp := phase.NewBasePhase(version, opts...)
	bp.Log = bp.Log.With("phase", "cleanup")
	return &Phase{BasePhase: bp}
}

// Execute runs the cleanup steps selected by opts.Kind. Individual step
// failures are accumulated and returned as a joined error; a partial run
// still attempts the remaining steps.
func (p *Phase) Execute(ctx context.Context, opts *Options) error {
	return execute(ctx, opts, p.Log)
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

// cleanupTracker buffers per-step errors and the names of failed subsystems
// for the final summary step. Orchestrator.Run does not propagate NonFatal
// step errors; the summary step returns errors.Join(t.errs...) so callers
// receive a joined error when one or more subsystem cleanups fail. Safe
// without a mutex because Orchestrator.Run executes steps serially.
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

func (t *cleanupTracker) failedNames() []string {
	return t.names
}

func execute(ctx context.Context, opts *Options, logger *slog.Logger) error {
	if opts.Kind == "" {
		return &errtypes.ConfigError{Msg: "cleanup kind not set"}
	}
	if err := opts.Kind.Validate(); err != nil {
		return err
	}
	defs := cleanupSteps(opts, logger)
	o := distribution.NewOrchestrator(distribution.BuildSteps(defs)...)
	o.SetLogger(logger)
	return o.Run(ctx)
}

func cleanupSummaryStep(opts *Options, t *cleanupTracker, logger *slog.Logger) distribution.StepDef {
	return distribution.StepDef{
		ID: StepCleanupSummary, Name: "cleanup summary",
		Desc: "printing cleanup summary", NonFatal: false,
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
		Desc: "removing generated ignition TLS certs", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeYes,
		AlreadyDone: func(_ context.Context) (bool, error) {
			return !system.DirExists(filepath.Join(opts.ProjectRoot, "certs", "ignition")), nil
		},
		Exec:    func(ctx context.Context) error { return IgnitionCerts(ctx, opts.ProjectRoot, logger) },
		OnError: t.onError("ignition-certs"),
	}
}

func cleanupSteps(opts *Options, logger *slog.Logger) []distribution.StepDef {
	t := &cleanupTracker{}

	workDirStep := distribution.StepDef{
		ID: StepCleanupWorkDir, Name: "cleanup work directory",
		Desc: "removing generated artifacts from work directory", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeNo,
		AlreadyDone: func(_ context.Context) (bool, error) {
			return !system.DirExists(opts.WorkDir), nil
		},
		Exec:    func(ctx context.Context) error { return WorkDirectory(ctx, opts.WorkDir, opts.PreserveConfig, logger) },
		OnError: t.onError("work directory"),
	}

	webServerStep := distribution.StepDef{
		ID: StepCleanupWebServer, Name: "cleanup web server",
		Desc: "removing ignition files from web server", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeYes,
		Exec:      func(ctx context.Context) error { return WebServer(ctx, opts.HTTPServerRoot, logger) },
		OnError:   t.onError("web server"),
	}

	haproxyStep := distribution.StepDef{
		ID: StepCleanupHAProxy, Name: "cleanup haproxy",
		Desc: "stopping haproxy and removing its configuration", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeNo,
		AlreadyDone: func(ctx context.Context) (bool, error) {
			return !system.FileExists(opts.HAProxyConfig) && !system.IsServiceActive(ctx, "haproxy"), nil
		},
		Exec:    func(ctx context.Context) error { return HAProxy(ctx, opts.HAProxyConfig, opts.VIP, logger) },
		OnError: t.onError("haproxy"),
	}

	apacheStep := distribution.StepDef{
		ID: StepCleanupApache, Name: "cleanup apache",
		Desc: "stopping apache httpd service", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeYes,
		Exec:      func(ctx context.Context) error { return Apache(ctx, logger) },
		OnError:   t.onError("apache"),
	}

	dnsmasqStep := distribution.StepDef{
		ID: StepCleanupDnsmasq, Name: "cleanup dnsmasq",
		Desc: "stopping dnsmasq and removing cluster dns configuration", NonFatal: true,
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

	terraformStep := distribution.StepDef{
		ID: StepCleanupTerraform, Name: "cleanup terraform",
		Desc: "removing generated terraform artifacts", NonFatal: true,
		ReRunSafe:   distribution.ReRunSafeNo,
		AlreadyDone: func(_ context.Context) (bool, error) { return terraformCleanupDone(opts) },
		Exec: func(ctx context.Context) error {
			if err := Terraform(ctx, opts.ProjectRoot, opts.TerraformEnv, logger); err != nil {
				return err
			}
			if !opts.PostDestroy || opts.TerraformEnv == "" {
				return nil
			}
			envDir := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv)
			tf := terraform.New(envDir, terraform.WithLogger(logger))
			if !tf.HasState() {
				_ = SafeRemoveWithLogger(ctx, filepath.Join(envDir, "terraform.tfstate"), "terraform state file", logger)
			}
			return nil
		},
		OnError: t.onError("terraform"),
	}

	packagesStep := distribution.StepDef{
		ID: StepCleanupPackages, Name: "cleanup packages",
		Desc: "removing installed packages and tool binaries", NonFatal: true,
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
