// Package system provides system-level utilities.
package system

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

const (
	dnsmasqConfigDir = "/etc/dnsmasq.d"
	dnsmasqService   = "dnsmasq"
)

// validConfigNameRegex matches safe config names: alphanumeric, hyphen, underscore only.
// Must start with alphanumeric and be 1-64 characters.
var validConfigNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// EnableDnsmasq enables dnsmasq to start on boot.
func EnableDnsmasq(ctx context.Context) error {
	return ManageService(ctx, ServiceEnable, dnsmasqService, "dnsmasq")
}

// RestartDnsmasq restarts the dnsmasq service.
func RestartDnsmasq(ctx context.Context) error {
	return ManageService(ctx, ServiceRestart, dnsmasqService, "dnsmasq")
}

// validateConfigName checks that a dnsmasq config name is safe.
// Only allows alphanumeric characters, hyphens, and underscores.
// Must start with alphanumeric and be 1-64 characters.
func validateConfigName(name string) error {
	if name == "" {
		return fmt.Errorf("config name cannot be empty")
	}
	if !validConfigNameRegex.MatchString(name) {
		return fmt.Errorf("config name must contain only alphanumeric characters, hyphens, and underscores, and start with alphanumeric")
	}
	return nil
}

// WriteDnsmasqConfig writes a dnsmasq configuration file.
// The config is written to /etc/dnsmasq.d/{name}.conf
func WriteDnsmasqConfig(ctx context.Context, name, content string) error {
	if err := validateConfigName(name); err != nil {
		return utils.WrapError("invalid config name", err)
	}

	configPath := filepath.Join(dnsmasqConfigDir, fmt.Sprintf("%s.conf", name))

	// Ensure config directory exists
	if err := MkdirAll(ctx, dnsmasqConfigDir, "dnsmasq config directory"); err != nil {
		return utils.WrapError("failed to create dnsmasq config directory", err)
	}

	// Write to temp file first
	tmpFile, err := os.CreateTemp("", "dnsmasq-*.conf")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	// Use a closure to ensure cleanup regardless of how we exit
	cleanup := func() { _ = os.Remove(tmpPath) }
	defer cleanup()

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return utils.WrapError("failed to write temp file", err)
	}
	if err := tmpFile.Close(); err != nil {
		return utils.WrapError("failed to close temp file", err)
	}

	// Copy to final location with elevation
	if err := CopyFileWithElevation(ctx, tmpPath, configPath, "dnsmasq config"); err != nil {
		return utils.WrapErrorf(err, "failed to copy config to %s", configPath)
	}

	// Set proper permissions
	if err := Chmod(ctx, configPath, "644", "dnsmasq config permissions"); err != nil {
		return utils.WrapError("failed to set config permissions", err)
	}

	return nil
}

// DnsmasqConfigPath returns the path to a dnsmasq config file.
// Returns empty string if the name is invalid.
func DnsmasqConfigPath(name string) string {
	if err := validateConfigName(name); err != nil {
		return ""
	}
	return filepath.Join(dnsmasqConfigDir, fmt.Sprintf("%s.conf", name))
}

// ═══════════════════════════════════════════════════════════════════════════════
// SYSTEM RESOLVER CONFIGURATION
// ═══════════════════════════════════════════════════════════════════════════════

// IsNetworkManagerActive checks if NetworkManager is available and running.
func IsNetworkManagerActive() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := exec.LookPath("nmcli"); err != nil {
		return false
	}
	return IsServiceActive("NetworkManager")
}

// getActiveConnection returns the name of the first active non-loopback connection.
func getActiveConnection(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show", "--active").Output()
	if err != nil {
		return "", utils.WrapError("failed to list network connections", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "lo" {
			return line, nil
		}
	}
	return "", fmt.Errorf("no active network connection found")
}

// validateDNSAddresses validates that all provided addresses are valid IPs.
func validateDNSAddresses(addresses []string) error {
	for _, addr := range addresses {
		if net.ParseIP(addr) == nil {
			return fmt.Errorf("invalid DNS address: %s", addr)
		}
	}
	return nil
}

// ConfigureSystemResolver configures the system to use localhost (dnsmasq) for DNS resolution.
// The fallbackDNS servers are used when dnsmasq cannot resolve a query.
func ConfigureSystemResolver(ctx context.Context, fallbackDNS []string) error {
	logger := utils.GetLogger()

	if !IsNetworkManagerActive() {
		logger.Warn("resolver: NetworkManager not active, skipping system resolver configuration")
		return nil
	}

	// Validate fallback DNS addresses
	if err := validateDNSAddresses(fallbackDNS); err != nil {
		return utils.WrapError("invalid fallback DNS configuration", err)
	}

	conn, err := getActiveConnection(ctx)
	if err != nil {
		return err
	}

	// Build DNS list: localhost first, then fallbacks
	dnsList := []string{"127.0.0.1"}
	dnsList = append(dnsList, fallbackDNS...)
	dnsConfig := strings.Join(dnsList, ",")

	logger.Info(fmt.Sprintf("resolver: configuring %s to use local dnsmasq", conn))

	// Set DNS servers and disable auto DNS from DHCP
	if err := runSudo("nmcli", "connection", "modify", conn, "ipv4.dns", dnsConfig, "ipv4.ignore-auto-dns", "yes"); err != nil {
		return utils.WrapError("failed to configure DNS for connection", err)
	}

	// Apply the configuration
	if err := runSudo("nmcli", "connection", "up", conn); err != nil {
		return utils.WrapError("failed to apply DNS configuration", err)
	}

	logger.Info("resolver: system configured to use local dnsmasq")
	return nil
}

// RestoreSystemResolver restores the system to use DHCP-provided DNS servers.
// Errors are logged as warnings but don't fail, since the goal is cleanup.
func RestoreSystemResolver(ctx context.Context) error {
	logger := utils.GetLogger()

	if !IsNetworkManagerActive() {
		return nil
	}

	conn, err := getActiveConnection(ctx)
	if err != nil {
		// No connection to restore - not an error during cleanup
		return nil
	}

	logger.Info(fmt.Sprintf("resolver: restoring DHCP DNS for %s", conn))

	// Clear custom DNS and re-enable auto DNS
	if err := runSudo("nmcli", "connection", "modify", conn, "ipv4.dns", "", "ipv4.ignore-auto-dns", "no"); err != nil {
		logger.Warn(fmt.Sprintf("resolver: failed to clear DNS settings: %v", err))
	}

	// Apply the configuration
	if err := runSudo("nmcli", "connection", "up", conn); err != nil {
		logger.Warn(fmt.Sprintf("resolver: failed to apply DNS configuration: %v", err))
	}

	logger.Info("resolver: system DNS restored to DHCP")
	return nil
}

