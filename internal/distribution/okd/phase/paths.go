// Package phase provides shared base types and path utilities for OKD phases.
package phase

import (
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
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
)

// ExternalToolBinaries returns the names of tool binaries installed into
// DefaultBinDir by the setup phase. Declared here (not in setup/) so cleanup
// can remove the same set without importing setup.
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
// destroy, cleanup) embeds — command executor, logger, and the okdctl version
// string used for provenance in generated artifacts.
type BasePhase struct {
	Exec     *executor.Executor
	Log      *slog.Logger
	Version  string
	Recorder distribution.MetricsRecorder
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
	return func(p *BasePhase) { p.Log = l }
}

// WithRecorder attaches a MetricsRecorder. Nil is tolerated; phases pass
// p.Recorder to orchestrator.SetMetricsRecorder which normalises nil to nop.
func WithRecorder(rec distribution.MetricsRecorder) BasePhaseOption {
	return func(p *BasePhase) { p.Recorder = rec }
}

// NewBasePhase constructs a BasePhase tagged with the okdctl version and the
// supplied options. Nil-safe for logger (→ NopLogger) and exec (→ a fresh
// executor wired to the same logger).
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
	return p
}
