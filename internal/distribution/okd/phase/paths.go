// Package phase provides shared base types and path utilities for OKD phases.
package phase

import (
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

type BaseOptions struct {
	ProjectRoot  string
	WorkDir      string
	Debug        bool
	TerraformEnv string
}

const (
	DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
	DefaultHAProxyBackupPath = "/etc/haproxy/haproxy.cfg.backup"
	DefaultHTTPServerRoot    = "/var/www/html"
	DefaultBinDir            = "/usr/local/bin"
	DefaultDNSMasqConfigDir  = "/etc/dnsmasq.d"
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

func ClusterConfigDir(workDir string) string {
	return filepath.Join(workDir, "cluster-config")
}

func GetTerraformEnv(cfg *config.Config) string {
	if cfg.Deployment.TerraformEnv != "" {
		return cfg.Deployment.TerraformEnv
	}
	return "production"
}

type BasePhase struct {
	Exec    *executor.Executor
	Log     *slog.Logger
	Version string
}

func NewBasePhase(exec *executor.Executor, logger *slog.Logger, version string) BasePhase {
	if logger == nil {
		logger = logutil.NopLogger
	}
	if exec == nil {
		exec = executor.New(executor.WithLogger(logger))
	}
	return BasePhase{
		Exec:    exec,
		Log:     logger,
		Version: version,
	}
}
