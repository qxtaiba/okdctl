// Package paths provides shared path utilities and base types for OKD phases.
package paths

import (
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

type BaseOptions struct {
	ProjectRoot  string
	WorkDir      string
	Debug        bool
	TerraformEnv string
}

const (
	DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
	DefaultHTTPServerRoot    = "/var/www/html"
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
	Log     utils.Logger
	Version string
}

func NewBasePhase(exec *executor.Executor, logger utils.Logger, version string) BasePhase {
	if logger == nil {
		logger = utils.NoopLogger()
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
