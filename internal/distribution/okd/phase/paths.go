// Package phase hosts BasePhase and the cross-phase helpers shared by every
// OKD phase. New cross-phase helpers belong here, not in a specific phase
// package; on-disk layout paths live in internal/workspace.
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

// BaseOptions is the option set every phase's own Options embeds: project
// root, per-run workDir, and the Terraform environment directory name.
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

// Default paths for bastion-phase artifacts; Debian-family paths route
// through platform.OS helpers instead.
const (
	// DefaultHAProxyConfigPath is where HAProxy reads its live config.
	DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
	// HAProxyBackupSuffix backs two schemes: setup's pristine snapshot at
	// <cfg>+suffix (DefaultHAProxyBackupPath), and postinstall's rolling
	// <cfg>+suffix+".<ts>" backups. Restore prefers the newest timestamped
	// backup, falling back to pristine; cleanup purges both via HAProxyBackupGlob.
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

// HAProxyBackupGlob matches every backup artifact for configPath (pristine
// snapshot plus timestamped backups); ".backup.<ts>" sorts above ".backup",
// so slices.Max over the matches prefers a timestamped backup.
func HAProxyBackupGlob(configPath string) string {
	return configPath + HAProxyBackupSuffix + "*"
}

// ExternalToolBinaries returns the tool binaries setup installs into BinDir;
// declared here (not setup/) so cleanup can reuse the set without an import cycle.
func ExternalToolBinaries() []string {
	return []string{
		"yq",
		"helm",
		"sops",
	}
}

// OKDToolBinaries returns the OKD release binaries setup extracts into
// BinDir, for the same import-cycle reason as ExternalToolBinaries.
func OKDToolBinaries() []string {
	return []string{
		"openshift-install",
		"oc",
		"kubectl",
	}
}

// BasePhase is the shared state every phase embeds: executor, logger,
// metrics recorder, and progress reporter.
type BasePhase struct {
	Exec       *executor.Executor
	Log        *slog.Logger
	Recorder   distribution.MetricsRecorder
	Reporter   logutil.ProgressReporter
	StatusLine logutil.StatusLineReporter
}

// BasePhaseOption configures a BasePhase at construction time.
type BasePhaseOption func(*BasePhase)

// WithExecutor sets the subprocess executor; nil is tolerated, NewBasePhase
// materializes a fresh executor wired to the same logger.
func WithExecutor(exec *executor.Executor) BasePhaseOption {
	return func(p *BasePhase) { p.Exec = exec }
}

// WithLogger attaches the phase logger; nil resolves to NopLogger.
func WithLogger(l *slog.Logger) BasePhaseOption {
	return func(p *BasePhase) { p.Log = logutil.OrNop(l) }
}

// WithRecorder attaches a MetricsRecorder; nil is tolerated and normalised
// to nop by orchestrator.SetMetricsRecorder.
func WithRecorder(rec distribution.MetricsRecorder) BasePhaseOption {
	return func(p *BasePhase) { p.Recorder = rec }
}

// WithReporter sets the progress reporter; nil resolves to logutil.NopProgressReporter.
func WithReporter(r logutil.ProgressReporter) BasePhaseOption {
	return func(p *BasePhase) { p.Reporter = r }
}

// WithStatusLine sets the install monitor's status-line reporter; nil
// resolves to logutil.NopStatusLineReporter.
func WithStatusLine(r logutil.StatusLineReporter) BasePhaseOption {
	return func(p *BasePhase) { p.StatusLine = r }
}

// NewBasePhase constructs a BasePhase from opts, defaulting a nil
// logger/exec/reporter/status-line to NopLogger, a fresh executor wired to
// that logger, NopProgressReporter, and NopStatusLineReporter.
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
