// Package phase hosts BasePhase plus the cross-phase helpers (OcResourceExists,
// OcPollOutput) shared by the setup, install, postinstall, destroy, and
// cleanup phases. New cross-phase helpers belong here per CLAUDE.md
// §architecture-notes — not in a specific phase package — to keep the import
// graph one-directional; on-disk layout paths live in internal/workspace.
package phase

import (
	"log/slog"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// BaseOptions is the common option set every phase's own Options embeds —
// the project checkout root, per-run workDir, and the name of the
// directory under infrastructure/terraform/environments/ to use.
type BaseOptions struct {
	ProjectRoot  string
	WorkDir      string
	TerraformEnv string
}

// NewBaseOptions resolves the standard phase roots for projectRoot: the
// workspace work dir and the Terraform environment configured in cfg.
func NewBaseOptions(cfg *config.Config, projectRoot string) BaseOptions {
	return BaseOptions{
		ProjectRoot:  projectRoot,
		WorkDir:      workspace.WorkDir(projectRoot),
		TerraformEnv: cfg.TerraformEnvName(),
	}
}

// KubeAPIPort is the kube-apiserver port served by HAProxy and kube-vip.
const KubeAPIPort = 6443

// Default paths for artifacts the bastion phase code writes or removes.
// Values follow the stock RHEL-family layout; Debian-family paths are
// resolved through platform.OS helpers instead.
const (
	// DefaultHAProxyConfigPath is where HAProxy reads its live config.
	DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
	// HAProxyBackupSuffix is the single suffix shared by every haproxy
	// config backup artifact. Two schemes hang off it, on purpose: setup
	// keeps one fixed pristine snapshot at <cfg>+suffix
	// (DefaultHAProxyBackupPath) as ConfigureHAProxy's rollback source,
	// and postinstall writes rolling timestamped backups at
	// <cfg>+suffix+".<ts>" before removing the config. Restore prefers
	// the newest timestamped backup and falls back to the pristine
	// snapshot; cleanup purges both via HAProxyBackupGlob.
	HAProxyBackupSuffix = ".backup"
	// DefaultHAProxyBackupPath is the fixed pristine snapshot setup writes
	// before rewriting DefaultHAProxyConfigPath.
	DefaultHAProxyBackupPath = DefaultHAProxyConfigPath + HAProxyBackupSuffix
	// DefaultHTTPServerRoot is where the bastion's httpd serves ignition.
	DefaultHTTPServerRoot = "/var/www/html"
	// DefaultDNSMasqConfigDir is where per-cluster dnsmasq fragments live.
	DefaultDNSMasqConfigDir = "/etc/dnsmasq.d"
)

// HAProxyTimestampedBackupPath returns the rolling-backup path for
// configPath at time now. The 20060102-150405 stamp sorts lexicographically,
// so slices.Max over HAProxyBackupGlob matches selects the newest.
func HAProxyTimestampedBackupPath(configPath string, now time.Time) string {
	return configPath + HAProxyBackupSuffix + "." + now.Format("20060102-150405")
}

// HAProxyBackupGlob returns the glob matching every backup artifact for
// configPath: the fixed pristine snapshot and all timestamped backups.
// "<cfg>.backup.<ts>" sorts above "<cfg>.backup", so slices.Max over the
// matches prefers a timestamped backup over the pristine snapshot.
func HAProxyBackupGlob(configPath string) string {
	return configPath + HAProxyBackupSuffix + "*"
}

// ExternalToolBinaries returns the names of tool binaries setup installs
// into BinDir. Declared in phase/ (not setup/) so cleanup can reference the
// same set without creating a setup→cleanup import cycle.
func ExternalToolBinaries() []string {
	return []string{
		"yq",
		"helm",
		"sops",
	}
}

// OKDToolBinaries returns the OKD release binaries setup extracts into
// BinDir. Declared in phase/ (not setup/) for the same reason as
// ExternalToolBinaries: cleanup must remove exactly the set setup installs.
func OKDToolBinaries() []string {
	return []string{
		"openshift-install",
		"oc",
		"kubectl",
	}
}

// BasePhase is the shared state every phase (setup, install, postinstall,
// destroy, cleanup) embeds — command executor, logger, metrics recorder,
// and progress reporter.
type BasePhase struct {
	Exec       *executor.Executor
	Log        *slog.Logger
	Recorder   distribution.MetricsRecorder
	Reporter   logutil.ProgressReporter
	StatusLine logutil.StatusLineReporter
}

// BasePhaseOption configures a BasePhase at construction time.
type BasePhaseOption func(*BasePhase)

// WithExecutor sets the subprocess executor. Nil is tolerated; NewBasePhase
// materializes a fresh executor wired to the same logger.
func WithExecutor(exec *executor.Executor) BasePhaseOption {
	return func(p *BasePhase) { p.Exec = exec }
}

// WithLogger attaches the phase logger. Nil resolves to NopLogger.
func WithLogger(l *slog.Logger) BasePhaseOption {
	return func(p *BasePhase) { p.Log = logutil.OrNop(l) }
}

// WithRecorder attaches a MetricsRecorder. Nil is tolerated; phases pass
// p.Recorder to orchestrator.SetMetricsRecorder which normalises nil to nop.
func WithRecorder(rec distribution.MetricsRecorder) BasePhaseOption {
	return func(p *BasePhase) { p.Recorder = rec }
}

// WithReporter sets the progress reporter for long-running phase operations.
// Nil resolves to logutil.NopProgressReporter.
func WithReporter(r logutil.ProgressReporter) BasePhaseOption {
	return func(p *BasePhase) { p.Reporter = r }
}

// WithStatusLine sets the updatable status-line reporter used by the install
// monitor. Nil resolves to logutil.NopStatusLineReporter.
func WithStatusLine(r logutil.StatusLineReporter) BasePhaseOption {
	return func(p *BasePhase) { p.StatusLine = r }
}

// NewBasePhase constructs a BasePhase from the supplied options. Nil-safe
// for logger (→ NopLogger), exec (→ fresh executor wired to the same
// logger), and reporter (→ NopProgressReporter).
func NewBasePhase(opts ...BasePhaseOption) BasePhase {
	p := BasePhase{}
	for _, opt := range opts {
		opt(&p)
	}
	if p.Log == nil {
		p.Log = logutil.NopLogger
	}
	if p.Exec == nil {
		p.Exec = executor.New(executor.WithLogger(p.Log))
	}
	if p.Reporter == nil {
		p.Reporter = logutil.NopProgressReporter
	}
	if p.StatusLine == nil {
		p.StatusLine = logutil.NopStatusLineReporter
	}
	return p
}
