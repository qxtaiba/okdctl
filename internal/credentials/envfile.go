package credentials

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// loadOnce guards against concurrent or repeated calls to LoadEnvFile.
// The global process environment is shared mutable state: without this
// guard, a "getenv empty → setenv" sequence races against any other
// goroutine touching the same key. The .env file is intentionally a
// per-process artefact — callers that pass a different path on a second
// invocation silently get the first path's result, which is preferred
// over letting multiple sources mutate the environment.
var (
	loadOnce   sync.Once
	loadErr    error
	loadedPath string
)

// EnvFilePath derives the .env path from a config path
// (e.g. "okdctl.yaml" becomes "okdctl.env").
func EnvFilePath(configPath string) string {
	if strings.HasSuffix(configPath, ".yaml") {
		return strings.TrimSuffix(configPath, ".yaml") + ".env"
	}
	if strings.HasSuffix(configPath, ".yml") {
		return strings.TrimSuffix(configPath, ".yml") + ".env"
	}
	return configPath + ".env"
}

// WriteEnvFile persists credentials in KEY=VALUE format compatible with
// standard .env tooling, with 0600 permissions.
func WriteEnvFile(path string, creds *ProxmoxCredentials) error {
	lines := []string{
		"# Proxmox credentials (managed by okdctl)",
		"# This file has restricted permissions (0600). Do not commit to git.",
	}

	if creds.Endpoint != "" {
		lines = append(lines, "PROXMOX_VE_ENDPOINT="+creds.Endpoint)
	}
	if creds.Username != "" {
		lines = append(lines, "PROXMOX_VE_USERNAME="+creds.Username)
	}
	if len(creds.Password) > 0 {
		// On-disk .env format is plain text — string() conversion here is
		// unavoidable. The in-memory []byte can still be wiped via Zeroize.
		lines = append(lines, "PROXMOX_VE_PASSWORD="+string(creds.Password))
	}
	if len(creds.APIToken) > 0 {
		lines = append(lines, "PROXMOX_VE_API_TOKEN="+string(creds.APIToken))
	}
	if creds.Insecure {
		lines = append(lines, "PROXMOX_VE_INSECURE=true")
	}

	content := strings.Join(lines, "\n") + "\n"
	return system.AtomicWrite(path, []byte(content), 0o600)
}

// LoadEnvFile loads a .env file into the process environment.
// Already-set variables are NOT overwritten (shell env takes precedence).
// Missing files are silently ignored.
//
// LoadEnvFile is guarded by a sync.Once: the underlying work runs at most
// once per process regardless of how many times (or with which paths) it
// is invoked. Subsequent calls return the first call's error. This is
// intentional — mutating os.Environ from multiple sources is a footgun,
// and the .env file is a per-process resource.
func LoadEnvFile(path string) error {
	loadOnce.Do(func() {
		loadedPath = path
		loadErr = loadEnvFileOnce(path)
	})
	if loadedPath != path {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("LoadEnvFile already called with %q; cannot reload from %q", loadedPath, path)}
	}
	return loadErr
}

// loadEnvFileOnce contains the actual load logic. It is not safe to call
// concurrently and must only be invoked via the LoadEnvFile sync.Once.
func loadEnvFileOnce(path string) error {
	// Refuse to load a .env that any other user can read. Proxmox tokens
	// end up in here — a world-readable file defeats the whole point of
	// moving secrets out of YAML. The permission check MUST happen before
	// godotenv opens the file: once its contents are in our address space,
	// the whole point of this check is to detect a file that other
	// processes may already have been able to read.
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // missing .env is not an error
		}
		return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to stat env file %s", path), Err: err}
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return &errtypes.AuthError{
			Msg: fmt.Sprintf(".env file %s has insecure permissions %#o; run 'chmod 600 %s' to fix", path, perm, path),
			Err: os.ErrPermission,
		}
	}

	// godotenv.Load does not overwrite already-set env vars, matching our
	// "shell takes precedence" contract.
	if err := godotenv.Load(path); err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to load env file %s", path), Err: err}
	}
	return nil
}
