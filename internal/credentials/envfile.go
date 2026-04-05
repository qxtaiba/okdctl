package credentials

import (
	"bufio"
	"os"
	"strings"
	"sync"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// loadOnce guards against concurrent or repeated calls to LoadEnvFile.
// The global process environment is shared mutable state: without this
// guard, a "getenv empty → setenv" sequence races against any other
// goroutine touching the same key. The .env file is intentionally a
// per-process artefact — callers that pass a different path on a second
// invocation silently get the first path's result, which is preferred
// over letting multiple sources mutate the environment.
var (
	loadOnce sync.Once
	loadErr  error
)

// EnvFilePath derives the .env path from a config path
// (e.g. "openshitctl.yaml" becomes "openshitctl.env").
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
	var lines []string
	lines = append(lines, "# Proxmox credentials (managed by openshitctl)")
	lines = append(lines, "# This file has restricted permissions (0600) — do not commit to git.")

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
	return os.WriteFile(path, []byte(content), 0600)
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
		loadErr = loadEnvFileOnce(path)
	})
	return loadErr
}

// loadEnvFileOnce contains the actual load logic. It is not safe to call
// concurrently and must only be invoked via the LoadEnvFile sync.Once.
func loadEnvFileOnce(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // missing .env is not an error
		}
		return err
	}
	defer f.Close() //nolint:errcheck // read-only file

	// Refuse to load a .env that any other user can read. Proxmox tokens
	// end up in here — a world-readable file defeats the whole point of
	// moving secrets out of YAML.
	fi, err := f.Stat()
	if err != nil {
		return utils.WrapErrorf(err, "failed to stat .env file %s", path)
	}
	if perm := fi.Mode().Perm(); perm&0077 != 0 {
		return utils.WrapErrorf(
			os.ErrPermission,
			".env file %s has insecure permissions %#o; run 'chmod 600 %s' to fix",
			path, perm, path,
		)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Only set if not already present — shell env wins
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
