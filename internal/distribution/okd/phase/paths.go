// Package phase hosts BasePhase plus the cross-phase helpers (OcResourceExists,
// OcPollOutput, NodeRole, ConditionStatus, VMState, SSHRunArgv) shared by the
// setup, install, postinstall, destroy, and cleanup phases. New cross-phase
// helpers belong here per CLAUDE.md §architecture-notes — not in a specific
// phase package — to keep the import graph one-directional.
package phase

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// BaseOptions is the common option set every phase's own Options embeds —
// the project checkout root, per-run workDir, Debug flag, and the terraform
// environment name (production|staging|...).
type BaseOptions struct {
	ProjectRoot  string
	WorkDir      string
	Debug        bool
	TerraformEnv string
}

// KubeAPIPort is the kube-apiserver port served by HAProxy and kube-vip.
const KubeAPIPort = 6443

// Default paths for artifacts the bastion phase code writes or removes.
// Values follow the stock RHEL-family layout; Debian-family paths are
// resolved through platform.OS helpers instead.
const (
	// DefaultHAProxyConfigPath is where HAProxy reads its live config.
	DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
	// DefaultHAProxyBackupPath is the reboot-safe snapshot setup writes
	// before rewriting DefaultHAProxyConfigPath.
	DefaultHAProxyBackupPath = "/etc/haproxy/haproxy.cfg.backup"
	// DefaultHTTPServerRoot is where the bastion's httpd serves ignition.
	DefaultHTTPServerRoot = "/var/www/html"
	// DefaultBinDir is where setup installs okd/terraform/yq/helm/sops.
	DefaultBinDir = "/usr/local/bin"
	// DefaultDNSMasqConfigDir is where per-cluster dnsmasq fragments live.
	DefaultDNSMasqConfigDir = "/etc/dnsmasq.d"
	// DefaultProxmoxISODir is the default Proxmox-managed path where downloaded
	// CoreOS ISOs are uploaded via scp and referenced by `qm importdisk`.
	DefaultProxmoxISODir = "/var/lib/vz/template/iso"
)

// ResolveBinDir returns the tool-install directory: OKDCTL_BIN_DIR env >
// cfg.Deployment.BinDir > DefaultBinDir. cfg may be nil; non-absolute values
// fall through. Paths are cleaned but `..` traversal is not rejected.
func ResolveBinDir(cfg *config.Config) string {
	if v := envBinDir(); v != "" {
		return v
	}
	if cfg != nil && cfg.Deployment.BinDir != "" {
		if dir, ok := validateAndClean(cfg.Deployment.BinDir); ok {
			return dir
		}
	}
	return DefaultBinDir
}

// PreflightBinDir returns the env-only bin dir resolution; the config is not
// yet parsed when main.preflight runs.
func PreflightBinDir() string {
	if v := envBinDir(); v != "" {
		return v
	}
	return DefaultBinDir
}

// BinDirOrDefault returns s when non-empty, else DefaultBinDir.
// Scaffolding: together with PreflightBinDir and ResolveBinDir this forms
// the three-function bin-dir-resolution surface; each function consults a
// different input source (struct field, env+config, env-only). Call sites
// in setup and cleanup use BinDirOrDefault as defense-in-depth — the field
// is already populated by ResolveBinDir at construction, but the explicit
// fallback documents that zero-value is safe and makes the resolution path
// auditable at each call site without tracing back to the constructor.
func BinDirOrDefault(s string) string {
	if s == "" {
		return DefaultBinDir
	}
	return s
}

func envBinDir() string {
	v := os.Getenv("OKDCTL_BIN_DIR")
	if v == "" {
		return ""
	}
	if dir, ok := validateAndClean(v); ok {
		return dir
	}
	return ""
}

func validateAndClean(raw string) (string, bool) {
	expanded := system.ExpandPath(raw)
	if err := config.ValidateBinDir(expanded); err != nil {
		return "", false
	}
	return filepath.Clean(expanded), true
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

// ClusterConfigDir returns the path where openshift-install writes its
// install-config.yaml and the generated kubeconfig/auth bundle.
func ClusterConfigDir(workDir string) string {
	return filepath.Join(workDir, "cluster-config")
}

// GetTerraformEnv returns the active terraform environment name from the
// config, defaulting to "production" when unset.
func GetTerraformEnv(cfg *config.Config) string {
	if cfg.Deployment.TerraformEnv != "" {
		return cfg.Deployment.TerraformEnv
	}
	return "production"
}

// BasePhase is the shared state every phase (setup, install, postinstall,
// destroy, cleanup) embeds — command executor, logger, okdctl version
// string used for provenance in generated artifacts, and progress reporter.
type BasePhase struct {
	Exec     *executor.Executor
	Log      *slog.Logger
	Version  string
	Recorder distribution.MetricsRecorder
	Reporter logutil.ProgressReporter
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

// WithVersion sets the okdctl version tag embedded in generated artifacts.
func WithVersion(v string) BasePhaseOption {
	return func(p *BasePhase) { p.Version = v }
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

// NewBasePhase constructs a BasePhase tagged with the okdctl version and the
// supplied options. Nil-safe for logger (→ NopLogger), exec (→ fresh executor
// wired to the same logger), and reporter (→ NopProgressReporter).
func NewBasePhase(version string, opts ...BasePhaseOption) BasePhase {
	p := BasePhase{Version: version}
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
	return p
}
