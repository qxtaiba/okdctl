package config

import (
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/system"
)

// DefaultBinDir is where setup installs okd/terraform/yq/helm/sops.
const DefaultBinDir = "/usr/local/bin"

// ResolveBinDir returns the tool-install directory, preferring OKDCTL_BIN_DIR
// over cfg.Deployment.BinDir over DefaultBinDir. cfg may be nil; non-absolute
// or `..`-containing values fall through to the next source.
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

// PreflightBinDir returns OKDCTL_BIN_DIR or DefaultBinDir; the config is not
// parsed yet when main.preflight runs.
func PreflightBinDir() string {
	if v := envBinDir(); v != "" {
		return v
	}
	return DefaultBinDir
}

// BinDirOrDefault returns s when non-empty, else DefaultBinDir. Defense in
// depth: call sites already pass a value resolved by ResolveBinDir.
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
