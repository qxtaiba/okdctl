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
	"slices"
	"strings"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/system"
)

const dnsmasqService = "dnsmasq"

// dnsmasqConfigDir is the directory for per-cluster dnsmasq fragments.
// Tests override this var to redirect writes to a t.TempDir().
var dnsmasqConfigDir = phase.DefaultDNSMasqConfigDir

// validateDnsmasqConfigFn and restartDnsmasqFn are package-level vars so
// tests can inject fakes without a real dnsmasq binary on PATH.
// resolvedConf is the systemd-resolved drop-in written by ConfigureSystemResolver.
// Tests override this var to redirect operations to a t.TempDir().
var resolvedConf = "/etc/systemd/resolved.conf.d/dnsmasq.conf"

var (
	validateDnsmasqConfigFn = ValidateDnsmasqConfig
	restartDnsmasqFn        = RestartDnsmasq
	// removeAllFn is the os.RemoveAll indirection used by RestoreSystemResolver.
	// Tests inject a failing func to cover the logged-but-not-propagated error path.
	removeAllFn = os.RemoveAll
)

var validConfigNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// validConnectionNameRegex is an allowlist for nmcli connection names. It
// accepts the realistic NetworkManager name space (alphanumerics, space,
// dot, underscore, slash, colon for interface aliases like br0:1, hyphen,
// up to 128 chars) and rejects everything else, including all shell
// metacharacters not enumerated.
var validConnectionNameRegex = regexp.MustCompile(`^[A-Za-z0-9 ._/:-]{1,128}$`)

// EnableDnsmasq enables and starts the dnsmasq service.
func EnableDnsmasq(ctx context.Context) error {
	return system.ManageService(ctx, system.ServiceEnable, dnsmasqService)
}

// RestartDnsmasq restarts the dnsmasq service.
func RestartDnsmasq(ctx context.Context) error {
	return system.ManageService(ctx, system.ServiceRestart, dnsmasqService)
}

// ValidateDnsmasqConfig runs "dnsmasq --test" to verify the on-disk config.
// stderr is captured so the returned error carries dnsmasq's actual
// syntax-error message, not just "exit status 1".
func ValidateDnsmasqConfig(ctx context.Context) error {
	return system.RunCaptured(ctx, "dnsmasq", "--test")
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

// writeDnsmasqConfig writes content to /etc/dnsmasq.d/<name>.conf. An
// existing file is copied to <path>.backup first so validateAndRestartDnsmasq
// can roll back on failure.
func writeDnsmasqConfig(ctx context.Context, name, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateConfigName(name); err != nil {
		return fmt.Errorf("invalid config name: %w", err)
	}

	configPath := filepath.Join(dnsmasqConfigDir, fmt.Sprintf("%s.conf", name))

	if system.FileExists(configPath) {
		backupPath := configPath + ".backup"
		if err := system.CopyFile(configPath, backupPath); err != nil {
			return fmt.Errorf("failed to back up config %s: %w", configPath, err)
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := system.AtomicWriteString(configPath, content, 0o644); err != nil {
		return fmt.Errorf("failed to write config %s: %w", configPath, err)
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

// validateConnectionName accepts only names matching validConnectionNameRegex.
// The allowlist fails-closed: any nmcli output containing characters outside
// the realistic NetworkManager name space is rejected, including every shell
// metacharacter (the previous denylist could grow stale).
func validateConnectionName(name string) error {
	if name == "" {
		return fmt.Errorf("connection name must not be empty")
	}
	if !validConnectionNameRegex.MatchString(name) {
		return fmt.Errorf("connection name %q does not match allowed character set", name)
	}
	return nil
}

func getActiveConnection(ctx context.Context) (string, error) {
	out, err := system.OutputCaptured(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show", "--active")
	if err != nil {
		return "", fmt.Errorf("failed to list network connections: %w", err)
	}

	for line := range strings.Lines(string(out)) {
		line = strings.TrimSpace(line)
		if line == "" || line == "lo" {
			continue
		}
		if err := validateConnectionName(line); err != nil {
			return "", fmt.Errorf("active connection name rejected: %w", err)
		}
		return line, nil
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

		dnsList := slices.Concat([]string{"127.0.0.1"}, fallbackDNS)
		dnsConfig := strings.Join(dnsList, ",")

		logger.Info("resolver: configuring connection to use local dnsmasq", "conn", conn)

		if err := system.RunCaptured(ctx, "nmcli", "connection", "modify", conn, "ipv4.dns", dnsConfig, "ipv4.ignore-auto-dns", "yes"); err != nil {
			return fmt.Errorf("failed to configure DNS for connection: %w", err)
		}

		if err := system.RunCaptured(ctx, "nmcli", "connection", "up", conn); err != nil {
			return fmt.Errorf("failed to apply DNS configuration: %w", err)
		}

		logger.Info("resolver: system configured to use local dnsmasq")
		return nil
	}

	// Fall back to systemd-resolved.
	if system.IsServiceActive(ctx, "systemd-resolved") {
		logger.Info("resolver: configuring systemd-resolved to use dnsmasq")
		confDir := "/etc/systemd/resolved.conf.d"
		confPath := confDir + "/dnsmasq.conf"
		confContent := "[Resolve]\nDNS=127.0.0.1\nDomains=~.\n"
		if err := os.MkdirAll(confDir, 0o755); err != nil {
			return fmt.Errorf("failed to create resolved.conf.d: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		tmpPath, err := system.WriteTempFile("resolved-conf", 0o644, func(f *os.File) error {
			_, err := f.WriteString(confContent)
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to write dnsmasq.conf: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		defer func() { _ = os.Remove(tmpPath) }()
		if err := system.CopyFile(tmpPath, confPath); err != nil {
			return fmt.Errorf("failed to install dnsmasq.conf: %w", err)
		}
		return system.RunCaptured(ctx, "systemctl", "restart", "systemd-resolved")
	}

	logger.Warn("resolver: neither NetworkManager nor systemd-resolved found, skipping system resolver configuration")
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

		logger.Info("resolver: restoring DHCP DNS", "conn", conn)

		if err := system.RunCaptured(ctx, "nmcli", "connection", "modify", conn, "ipv4.dns", "", "ipv4.ignore-auto-dns", "no"); err != nil {
			logger.Warn("resolver: failed to clear DNS settings", "err", err)
		}

		if err := system.RunCaptured(ctx, "nmcli", "connection", "up", conn); err != nil {
			logger.Warn("resolver: failed to apply DNS configuration", "err", err)
		}

		logger.Info("resolver: system DNS restored to DHCP")
		return nil
	}

	// Clean up systemd-resolved drop-in if it exists.
	if system.FileExists(resolvedConf) {
		logger.Info("resolver: removing systemd-resolved dnsmasq configuration")
		if err := removeAllFn(resolvedConf); err != nil {
			logger.Warn("resolver: failed to remove", "path", resolvedConf, "err", err)
		}
		if system.IsServiceActive(ctx, "systemd-resolved") {
			_ = system.ManageService(ctx, system.ServiceRestart, "systemd-resolved")
		}
		logger.Info("resolver: systemd-resolved configuration restored")
	}

	return nil
}
