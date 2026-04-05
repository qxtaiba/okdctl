package credentials

import (
	"bufio"
	"os"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
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
func LoadEnvFile(path string) error {
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
