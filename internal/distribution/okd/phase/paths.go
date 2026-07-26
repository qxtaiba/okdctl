// Package phase hosts BasePhase plus the cross-phase helpers (OcResourceExists,
// OcPollOutput, path layout) shared by the setup, install, postinstall,
// destroy, and cleanup phases. New cross-phase helpers belong here per
// CLAUDE.md §architecture-notes — not in a specific phase package — to keep
// the import graph one-directional.
package phase

import (
	"log/slog"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// BaseOptions is the common option set every phase's own Options embeds —
// the project checkout root, per-run workDir, and the name of the
// directory under infrastructure/terraform/environments/ to use.
type BaseOptions struct {
	ProjectRoot  string
	WorkDir      string
	TerraformEnv string
}

// WorkDirName re-exports system.WorkDirName so every phase package and cli
// build the per-project workdir path from one source of truth.
const WorkDirName = system.WorkDirName

// KubeAPIPort is the kube-apiserver port served by HAProxy and kube-vip.
const KubeAPIPort = 6443

// BootstrapStateSentinelFile is the auto-loaded tfvars override postinstall
// writes after the bootstrap VM is destroyed. Terraform loads *.auto.tfvars.json
// after terraform.tfvars, so cleanup and setup must remove this file before any
// subsequent deploy so bootstrap_enabled=true takes effect again.
const BootstrapStateSentinelFile = "bootstrap-state.auto.tfvars.json"

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

// ClusterConfigDir re-exports system.ClusterConfigDir so phase code keeps a
// single import for cluster layout paths; non-phase callers use the system
// home directly.
func ClusterConfigDir(workDir string) string {
	return system.ClusterConfigDir(workDir)
}

// GetTerraformEnv re-exports Config.TerraformEnvName for phase callers.
func GetTerraformEnv(cfg *config.Config) string {
	return cfg.TerraformEnvName()
}

// TerraformEnvDir re-exports system.TerraformEnvDir so phase callers build the
// Terraform environment path from the same source of truth as
// internal/infrastructure/proxmox (which cannot import phase; see roadmap B1).
func TerraformEnvDir(projectRoot, env string) string {
	return system.TerraformEnvDir(projectRoot, env)
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
