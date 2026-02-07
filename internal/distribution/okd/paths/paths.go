// Package paths provides shared path utilities and base types for OKD phases.
// This package has no dependencies on phase packages to avoid circular imports.
package paths

import (
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

// BaseOptions contains common options for all OKD phases.
// Embed this in phase-specific Options structs to avoid duplication.
type BaseOptions struct {
	// ProjectRoot is the root of the project for accessing templates.
	ProjectRoot string

	// WorkDir is the working directory for OKD installation files.
	WorkDir string

	// Debug enables verbose debug logging.
	Debug bool

	// TerraformEnv is the Terraform environment name.
	TerraformEnv string
}

// Path constants for OKD configuration files.
const (
	// DefaultHAProxyConfigPath is the default path to the HAProxy configuration file.
	DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
	// DefaultHTTPServerRoot is the default web server root directory for ignition files.
	DefaultHTTPServerRoot = "/var/www/html"
)

// ClusterConfigDir returns the path to the cluster configuration directory.
func ClusterConfigDir(workDir string) string {
	return filepath.Join(workDir, "cluster-config")
}

// GetTerraformEnv returns the terraform environment name, defaulting to "production".
func GetTerraformEnv(cfg *config.Config) string {
	if cfg.Deployment.TerraformEnv != "" {
		return cfg.Deployment.TerraformEnv
	}
	return "production"
}

// BasePhase provides shared functionality for all OKD phase implementations.
// Embed this struct in phase structs to get common executor, logger, and version fields.
type BasePhase struct {
	Exec    *executor.Executor
	Log     logging.Logger
	Version string
}

// NewBasePhase creates a new BasePhase with the given dependencies.
// If exec is nil, a new default executor is created.
// If logger is nil, a noop logger is used.
func NewBasePhase(exec *executor.Executor, logger logging.Logger, version string) BasePhase {
	if exec == nil {
		exec = executor.New()
	}
	if logger == nil {
		logger = logging.NoopLogger()
	}
	return BasePhase{
		Exec:    exec,
		Log:     logger,
		Version: version,
	}
}

// Executor returns the phase's command executor.
func (b *BasePhase) Executor() *executor.Executor {
	return b.Exec
}

// Logger returns the phase's logger.
func (b *BasePhase) Logger() logging.Logger {
	return b.Log
}

// LogInfo logs an info message.
func (b *BasePhase) LogInfo(msg string) {
	b.Log.Info(msg)
}

// LogWarn logs a warning message.
func (b *BasePhase) LogWarn(msg string) {
	b.Log.Warn(msg)
}

// LogError logs an error message.
func (b *BasePhase) LogError(msg string) {
	b.Log.Error(msg)
}
