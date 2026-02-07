package credentials

import (
	"bufio"
	"os"
	"strings"
)

// EnvFilePath returns the .env file path corresponding to a YAML config path.
// For example, "openshitctl.yaml" becomes "openshitctl.env".
func EnvFilePath(configPath string) string {
	if strings.HasSuffix(configPath, ".yaml") {
		return strings.TrimSuffix(configPath, ".yaml") + ".env"
	}
	if strings.HasSuffix(configPath, ".yml") {
		return strings.TrimSuffix(configPath, ".yml") + ".env"
	}
	return configPath + ".env"
}

// WriteEnvFile writes Proxmox credentials to a .env file with 0600 permissions.
// The file uses KEY=VALUE format compatible with standard .env tooling.
func WriteEnvFile(path string, creds *ProxmoxCredentials) error {
	var lines []string
	lines = append(lines, "# Proxmox credentials (managed by openshitctl)")
	lines = append(lines, "# This file has restricted permissions (0600) — do not commit to git.")

	if creds.Username != "" {
		lines = append(lines, "PROXMOX_VE_USERNAME="+creds.Username)
	}
	if creds.Password != "" {
		lines = append(lines, "PROXMOX_VE_PASSWORD="+creds.Password)
	}
	if creds.APIToken != "" {
		lines = append(lines, "PROXMOX_VE_API_TOKEN="+creds.APIToken)
	}
	if creds.Insecure {
		lines = append(lines, "PROXMOX_VE_INSECURE=true")
	}

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0600)
}

// LoadEnvFile reads KEY=VALUE pairs from a .env file and sets them as
// environment variables. Variables that are already set in the environment
// are NOT overwritten, ensuring shell env vars always take precedence.
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
