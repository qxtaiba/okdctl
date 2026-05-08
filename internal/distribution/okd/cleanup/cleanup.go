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
	"strings"

	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
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
	for _, v := range ValidKinds() {
		if k == v {
			return true
		}
	}
	return false
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
}

// cleanupConfig carries the resolved logger applied via WithLogger.
type cleanupConfig struct{ logger *slog.Logger }

// Option configures optional knobs on an Execute call.
type Option func(*cleanupConfig)

// WithLogger injects a structured logger for the cleanup run; nil falls back to logutil.NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(c *cleanupConfig) { c.logger = logutil.OrNop(l) }
}

// Phase drives a cleanup run.
type Phase struct {
	phase.BasePhase
}

// New constructs a cleanup Phase bound to exec/logger and the okdctl
// version tag. It mirrors the shape of setup/install/postinstall/destroy.
func New(exec *executor.Executor, logger *slog.Logger, version string) *Phase {
	phaseLogger := logutil.OrNop(logger).With("phase", "cleanup")
	return &Phase{
		BasePhase: phase.NewBasePhase(version, phase.WithExecutor(exec), phase.WithLogger(phaseLogger)),
	}
}

// Execute runs the cleanup steps selected by opts.Kind. Individual step
// failures are accumulated and returned as a joined error; a partial run
// still attempts the remaining steps.
func (p *Phase) Execute(ctx context.Context, opts *Options, options ...Option) error {
	cfg := &cleanupConfig{logger: logutil.NopLogger}
	for _, o := range options {
		o(cfg)
	}
	return execute(ctx, opts, cfg.logger)
}

// Step IDs for the cleanup phase, ordered as they execute within Full.
const (
	StepCleanupWorkDir   distribution.StepID = "cleanup-workdir"
	StepCleanupWebServer distribution.StepID = "cleanup-webserver"
	StepCleanupHAProxy   distribution.StepID = "cleanup-haproxy"
	StepCleanupApache    distribution.StepID = "cleanup-apache"
	StepCleanupDnsmasq   distribution.StepID = "cleanup-dnsmasq"
	StepCleanupTerraform distribution.StepID = "cleanup-terraform"
	StepCleanupPackages  distribution.StepID = "cleanup-packages"
	StepCleanupSummary   distribution.StepID = "cleanup-summary"
)

// cleanupTracker buffers per-step errors for the final summary step.
// Orchestrator.Run does not propagate NonFatal step errors; the summary step
// returns errors.Join(t.errs...) so callers receive a joined error when one
// or more subsystem cleanups fail. Safe without a mutex because
// Orchestrator.Run executes steps serially.
type cleanupTracker struct {
	errs []error
}

func (t *cleanupTracker) onError() func(error) {
	return func(err error) {
		t.errs = append(t.errs, err)
	}
}

func execute(ctx context.Context, opts *Options, logger *slog.Logger) error {
	if opts.Kind == "" {
		return &errtypes.ConfigError{Msg: "cleanup kind not set"}
	}
	if err := opts.Kind.Validate(); err != nil {
		return err
	}
	l := logger.With("phase", "cleanup")
	defs := cleanupSteps(opts, l)
	o := distribution.NewOrchestrator(distribution.BuildSteps(defs)...)
	o.SetLogger(l)
	return o.Run(ctx)
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
		OnError: t.onError(),
	}

	webServerStep := distribution.StepDef{
		ID: StepCleanupWebServer, Name: "cleanup web server",
		Desc: "removing ignition files from web server", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeYes,
		Exec:      func(ctx context.Context) error { return WebServer(ctx, opts.HTTPServerRoot, logger) },
		OnError:   t.onError(),
	}

	haproxyStep := distribution.StepDef{
		ID: StepCleanupHAProxy, Name: "cleanup haproxy",
		Desc: "stopping haproxy and removing its configuration", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeNo,
		AlreadyDone: func(ctx context.Context) (bool, error) {
			return !system.FileExists(opts.HAProxyConfig) && !system.IsServiceActive(ctx, "haproxy"), nil
		},
		Exec:    func(ctx context.Context) error { return HAProxy(ctx, opts.HAProxyConfig, opts.VIP, logger) },
		OnError: t.onError(),
	}

	apacheStep := distribution.StepDef{
		ID: StepCleanupApache, Name: "cleanup apache",
		Desc: "stopping apache httpd service", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeYes,
		Exec:      func(ctx context.Context) error { return Apache(ctx, logger) },
		OnError:   t.onError(),
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
				return false, err
			}
			return !system.FileExists(confPath), nil
		},
		Exec:    func(ctx context.Context) error { return Dnsmasq(ctx, opts.ClusterName, logger) },
		OnError: t.onError(),
	}

	terraformStep := distribution.StepDef{
		ID: StepCleanupTerraform, Name: "cleanup terraform",
		Desc: "removing generated terraform artifacts", NonFatal: true,
		ReRunSafe: distribution.ReRunSafeNo,
		AlreadyDone: func(_ context.Context) (bool, error) {
			if opts.TerraformEnv == "" {
				return false, nil
			}
			tfvars := filepath.Join(opts.ProjectRoot, "infrastructure", "terraform", "environments", opts.TerraformEnv, "terraform.tfvars")
			return !system.FileExists(tfvars), nil
		},
		Exec:    func(ctx context.Context) error { return Terraform(ctx, opts.ProjectRoot, opts.TerraformEnv, logger) },
		OnError: t.onError(),
	}

	packagesStep := distribution.StepDef{
		ID: StepCleanupPackages, Name: "cleanup packages",
		Desc: "removing installed packages and tool binaries", NonFatal: true,
		ReRunSafe:  distribution.ReRunSafeYes,
		SkipWhen:   func() bool { return !opts.RemovePackages },
		SkipReason: "package removal disabled",
		Exec:       func(ctx context.Context) error { return Packages(ctx, opts.BinDir, logger) },
		OnError:    t.onError(),
	}

	summaryStep := distribution.StepDef{
		ID: StepCleanupSummary, Name: "cleanup summary",
		Desc: "printing cleanup summary", NonFatal: false,
		ReRunSafe: distribution.ReRunSafeYes,
		Exec: func(_ context.Context) error {
			printSummary(opts, logger)
			return errors.Join(t.errs...)
		},
	}

	var defs []distribution.StepDef
	switch opts.Kind {
	case Full:
		defs = []distribution.StepDef{workDirStep, webServerStep, haproxyStep, apacheStep, dnsmasqStep, terraformStep, packagesStep}
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
