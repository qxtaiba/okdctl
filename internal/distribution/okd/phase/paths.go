// Package phase provides shared base types and path utilities for OKD phases.
package phase

import (
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
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
		logger = slog.New(slog.DiscardHandler)
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
