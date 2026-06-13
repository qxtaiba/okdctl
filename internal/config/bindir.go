package config

import (
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/system"
)

// DefaultBinDir is where setup installs okd/terraform/yq/helm/sops.
const DefaultBinDir = "/usr/local/bin"

// ResolveBinDir returns the tool-install directory: OKDCTL_BIN_DIR env >
// cfg.Deployment.BinDir > DefaultBinDir. cfg may be nil; non-absolute values
// fall through. Paths are cleaned but `..` traversal is not rejected. See
// BinDirOrDefault for the full three-function surface rationale.
func ResolveBinDir(cfg *Config) string {
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

// PreflightBinDir returns the env-only bin dir (OKDCTL_BIN_DIR > DefaultBinDir);
// the config is not yet parsed when main.preflight runs. See BinDirOrDefault
// for the full three-function surface rationale.
func PreflightBinDir() string {
	if v := envBinDir(); v != "" {
		return v
	}
	return DefaultBinDir
}

// BinDirOrDefault returns s when non-empty, else DefaultBinDir.
// Scaffolding (api:0139cb3f): together with PreflightBinDir and ResolveBinDir
// this forms the three-function bin-dir-resolution surface; each function
// consults a different input source (struct field, env+config, env-only). Call
// sites in setup and cleanup use BinDirOrDefault as defense-in-depth — the
// field is already populated by ResolveBinDir at construction, but the explicit
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
	if err := ValidateBinDir(expanded); err != nil {
		return "", false
	}
	return filepath.Clean(expanded), true
}
