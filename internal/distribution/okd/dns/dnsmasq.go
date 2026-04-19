package dns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/system"
)

const (
	dnsmasqConfigDir = phase.DefaultDNSMasqConfigDir
	dnsmasqService   = "dnsmasq"
)

var validConfigNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// EnableDnsmasq enables and starts the dnsmasq service.
func EnableDnsmasq(ctx context.Context) error {
	return system.ManageService(ctx, system.ServiceEnable, dnsmasqService, "dnsmasq")
}

// RestartDnsmasq restarts the dnsmasq service.
func RestartDnsmasq(ctx context.Context) error {
	return system.ManageService(ctx, system.ServiceRestart, dnsmasqService, "dnsmasq")
}

// ValidateDnsmasqConfig runs "dnsmasq --test" to verify the on-disk config.
func ValidateDnsmasqConfig(ctx context.Context) error {
	return exec.CommandContext(ctx, "dnsmasq", "--test").Run()
}

func validateConfigName(name string) error {
	if name == "" {
		return fmt.Errorf("config name cannot be empty")
	}
	if !validConfigNameRegex.MatchString(name) {
		return fmt.Errorf("config name must contain only alphanumeric characters, hyphens, and underscores, and start with alphanumeric")
	}
	return nil
}

// WriteDnsmasqConfig writes content to /etc/dnsmasq.d/<name>.conf. An
// existing file is copied to <path>.backup first so validateAndRestartDnsmasq
// can roll back on failure.
func WriteDnsmasqConfig(_ context.Context, name, content string) error {
	if err := validateConfigName(name); err != nil {
		return fmt.Errorf("invalid config name: %w", err)
	}

	configPath := filepath.Join(dnsmasqConfigDir, fmt.Sprintf("%s.conf", name))

	if err := os.MkdirAll(dnsmasqConfigDir, 0o755); err != nil {
		return fmt.Errorf("failed to create dnsmasq config directory: %w", err)
	}

	if system.FileExists(configPath) {
		backupPath := configPath + ".backup"
		if err := system.CopyFile(configPath, backupPath); err != nil {
			return fmt.Errorf("failed to back up config %s: %w", configPath, err)
		}
	}

	tmpPath, err := system.WriteTempFile("dnsmasq-*.conf", 0o644, func(f *os.File) error {
		_, err := f.WriteString(content)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if err := system.CopyFile(tmpPath, configPath); err != nil {
		return fmt.Errorf("failed to copy config to %s: %w", configPath, err)
	}

	return nil
}

// DnsmasqConfigPath returns the absolute path for the named drop-in config.
// The name is validated to reject path-traversal characters.
func DnsmasqConfigPath(name string) (string, error) {
	if err := validateConfigName(name); err != nil {
		return "", fmt.Errorf("invalid dnsmasq config name: %w", err)
	}
	return filepath.Join(dnsmasqConfigDir, fmt.Sprintf("%s.conf", name)), nil
}

// IsNetworkManagerActive reports whether NetworkManager is running on a
// Linux host with nmcli present. Returns false on non-Linux platforms.
func IsNetworkManagerActive(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := exec.LookPath("nmcli"); err != nil {
		return false
	}
	return system.IsServiceActive(ctx, "NetworkManager")
}

func getActiveConnection(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show", "--active").Output()
	if err != nil {
		return "", fmt.Errorf("failed to list network connections: %w", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "lo" {
			return line, nil
		}
	}
	return "", fmt.Errorf("no active network connection found")
}

func validateDNSAddresses(addresses []string) error {
	for _, addr := range addresses {
		if err := config.ValidateIP(addr); err != nil {
			return fmt.Errorf("invalid DNS address %s: %w", addr, err)
		}
	}
	return nil
}

// ConfigureSystemResolver configures the system to use localhost (dnsmasq) for DNS resolution,
// with the given fallbackDNS servers for queries dnsmasq cannot resolve.
//
// It tries NetworkManager first, then systemd-resolved, and logs a warning if neither is found.
func ConfigureSystemResolver(ctx context.Context, fallbackDNS []string, logger *slog.Logger) error {
	if err := validateDNSAddresses(fallbackDNS); err != nil {
		return fmt.Errorf("invalid fallback DNS configuration: %w", err)
	}

	// Prefer NetworkManager when available.
	if IsNetworkManagerActive(ctx) {
		conn, err := getActiveConnection(ctx)
		if err != nil {
			return err
		}

		dnsList := []string{"127.0.0.1"}
		dnsList = append(dnsList, fallbackDNS...)
		dnsConfig := strings.Join(dnsList, ",")

		logger.Info(fmt.Sprintf("resolver: configuring %s to use local dnsmasq", conn))

		if err := exec.CommandContext(ctx, "nmcli", "connection", "modify", conn, "ipv4.dns", dnsConfig, "ipv4.ignore-auto-dns", "yes").Run(); err != nil {
			return fmt.Errorf("failed to configure DNS for connection: %w", err)
		}

		if err := exec.CommandContext(ctx, "nmcli", "connection", "up", conn).Run(); err != nil {
			return fmt.Errorf("failed to apply DNS configuration: %w", err)
		}

		logger.Info("resolver: system configured to use local dnsmasq")
		return nil
	}

	// Fall back to systemd-resolved.
	if system.IsServiceActive(ctx, "systemd-resolved") {
		logger.Info("dns: configuring systemd-resolved to use dnsmasq")
		confDir := "/etc/systemd/resolved.conf.d"
		confPath := confDir + "/dnsmasq.conf"
		confContent := "[Resolve]\nDNS=127.0.0.1\nDomains=~.\n"
		if err := os.MkdirAll(confDir, 0o755); err != nil {
			return fmt.Errorf("failed to create resolved.conf.d: %w", err)
		}
		tmpPath, err := system.WriteTempFile("resolved-conf", 0o644, func(f *os.File) error {
			_, err := f.WriteString(confContent)
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to write dnsmasq.conf: %w", err)
		}
		defer func() { _ = os.Remove(tmpPath) }()
		if err := system.CopyFile(tmpPath, confPath); err != nil {
			return fmt.Errorf("failed to install dnsmasq.conf: %w", err)
		}
		return exec.CommandContext(ctx, "systemctl", "restart", "systemd-resolved").Run()
	}

	logger.Warn("dns: neither NetworkManager nor systemd-resolved found, skipping system resolver configuration")
	return nil
}

// RestoreSystemResolver undoes ConfigureSystemResolver: it clears the
// nmcli DNS override or removes the systemd-resolved drop-in. Failures are
// logged but do not abort cleanup.
func RestoreSystemResolver(ctx context.Context, logger *slog.Logger) error {
	if IsNetworkManagerActive(ctx) {
		conn, err := getActiveConnection(ctx)
		if err != nil {
			logger.Warn("resolver: could not detect active connection for restore", "err", err)
			return nil // best-effort restore; no active connection is non-fatal
		}

		logger.Info(fmt.Sprintf("resolver: restoring DHCP DNS for %s", conn))

		if err := exec.CommandContext(ctx, "nmcli", "connection", "modify", conn, "ipv4.dns", "", "ipv4.ignore-auto-dns", "no").Run(); err != nil {
			logger.Warn("resolver: failed to clear DNS settings", "err", err)
		}

		if err := exec.CommandContext(ctx, "nmcli", "connection", "up", conn).Run(); err != nil {
			logger.Warn("resolver: failed to apply DNS configuration", "err", err)
		}

		logger.Info("resolver: system DNS restored to DHCP")
		return nil
	}

	// Clean up systemd-resolved drop-in if it exists.
	const resolvedConf = "/etc/systemd/resolved.conf.d/dnsmasq.conf"
	if system.FileExists(resolvedConf) {
		logger.Info("resolver: removing systemd-resolved dnsmasq configuration")
		if err := os.RemoveAll(resolvedConf); err != nil {
			logger.Warn("resolver: failed to remove", "path", resolvedConf, "err", err)
		}
		if system.IsServiceActive(ctx, "systemd-resolved") {
			_ = system.ManageService(ctx, system.ServiceRestart, "systemd-resolved", "systemd-resolved")
		}
		logger.Info("resolver: systemd-resolved configuration restored")
	}

	return nil
}
