package dns

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/hostnet"
	"github.com/qxtaiba/okdctl/internal/system"
)

const dnsmasqService = "dnsmasq"

// dnsmasqConfigDir is overridden to t.TempDir() in tests.
var dnsmasqConfigDir = phase.DefaultDNSMasqConfigDir

var (
	// validateDnsmasqConfigFn/restartDnsmasqFn: package vars so tests can
	// inject fakes without a real dnsmasq binary.
	validateDnsmasqConfigFn = ValidateDnsmasqConfig
	restartDnsmasqFn        = RestartDnsmasq
	// isNetworkManagerActiveFn lets tests drive resolver paths on non-Linux
	// hosts, bypassing the runtime.GOOS gate.
	isNetworkManagerActiveFn = IsNetworkManagerActive
)

var validConfigNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// EnableDnsmasq enables dnsmasq at boot (systemctl enable, no --now) without starting it.
func EnableDnsmasq(ctx context.Context) error {
	return system.ManageService(ctx, system.ServiceEnable, dnsmasqService)
}

// RestartDnsmasq restarts dnsmasq to pick up a new config.
// Callers must run ValidateDnsmasqConfig first, or a broken config takes cluster DNS down.
func RestartDnsmasq(ctx context.Context) error {
	return system.ManageService(ctx, system.ServiceRestart, dnsmasqService)
}

// ValidateDnsmasqConfig runs "dnsmasq --test" to verify the on-disk config.
// stderr is captured so the error carries dnsmasq's syntax message, not just "exit status 1".
func ValidateDnsmasqConfig(ctx context.Context) error {
	return executor.RunCaptured(ctx, "dnsmasq", "--test")
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

// writeDnsmasqConfig writes to /etc/dnsmasq.d/<name>.conf, backing up any
// existing file for validateAndRestartDnsmasq's rollback.
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
			return fmt.Errorf("back up config %s: %w", configPath, err)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := system.AtomicWriteString(configPath, content, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", configPath, err)
	}

	return nil
}

// DnsmasqConfigPath returns the absolute path for the named drop-in config,
// rejecting path-traversal characters in name.
func DnsmasqConfigPath(name string) (string, error) {
	if err := validateConfigName(name); err != nil {
		return "", fmt.Errorf("invalid dnsmasq config name: %w", err)
	}
	return filepath.Join(dnsmasqConfigDir, fmt.Sprintf("%s.conf", name)), nil
}

// IsNetworkManagerActive reports whether NetworkManager is active on a Linux
// host with nmcli present (always false elsewhere).
func IsNetworkManagerActive(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := exec.LookPath("nmcli"); err != nil {
		return false
	}
	return system.IsServiceActive(ctx, "NetworkManager")
}

func validateDNSAddresses(addresses []string) error {
	for _, addr := range addresses {
		if err := config.ValidateIP(addr); err != nil {
			return fmt.Errorf("invalid DNS address %s: %w", addr, err)
		}
	}
	return nil
}

// ConfigureSystemResolver points system DNS at localhost (dnsmasq) through
// NetworkManager, using fallbackDNS for queries dnsmasq can't resolve; it
// only warns when NetworkManager is not active.
func ConfigureSystemResolver(ctx context.Context, fallbackDNS []string, logger *slog.Logger) error {
	if err := validateDNSAddresses(fallbackDNS); err != nil {
		return fmt.Errorf("invalid fallback DNS configuration: %w", err)
	}

	if isNetworkManagerActiveFn(ctx) {
		conn, err := hostnet.ActiveConnection(ctx)
		if err != nil {
			return err
		}

		// Captured before any mutation so a failed connection-up can revert;
		// fail here rather than half-apply with no way back.
		prevDNS, prevIgnore, err := captureConnDNS(ctx, conn)
		if err != nil {
			return fmt.Errorf("capture current DNS settings: %w", err)
		}

		dnsList := slices.Concat([]string{"127.0.0.1"}, fallbackDNS)

		logger.Info("resolver: configuring connection to use local dnsmasq", "conn", conn)

		if err := hostnet.OverrideConnectionDNS(ctx, conn, dnsList); err != nil {
			return fmt.Errorf("configure DNS for connection: %w", err)
		}

		if err := hostnet.ActivateConnection(ctx, conn); err != nil {
			// connection-up failed with DNS forced to 127.0.0.1; revert so a
			// reboot doesn't resurrect a dead resolver (ctx detached so Ctrl-C
			// can't kill the revert too).
			rCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resolverRestoreTimeout)
			defer cancel()
			if restoreErr := restoreConnDNS(rCtx, conn, prevDNS, prevIgnore); restoreErr != nil {
				return fmt.Errorf("apply DNS configuration: %w (profile %s was rewritten to 127.0.0.1 and reverting it also failed: %w)", err, conn, restoreErr)
			}
			return fmt.Errorf("apply DNS configuration: %w (profile %s reverted to previous DNS)", err, conn)
		}

		logger.Info("resolver: system configured to use local dnsmasq")
		return nil
	}

	logger.Warn("resolver: NetworkManager not active, skipping system resolver configuration")
	return nil
}

// resolverRestoreTimeout bounds the detached rollback call after a partial resolver mutation.
const resolverRestoreTimeout = 30 * time.Second

// captureConnDNS reads conn's ipv4.dns/ipv4.ignore-auto-dns so
// ConfigureSystemResolver can revert them if connection-up fails.
func captureConnDNS(ctx context.Context, conn string) (dns, ignoreAutoDNS string, err error) {
	dnsOut, err := executor.OutputCaptured(ctx, "nmcli", "-g", "ipv4.dns", "connection", "show", conn)
	if err != nil {
		return "", "", fmt.Errorf("read ipv4.dns for %s: %w", conn, err)
	}
	ignoreOut, err := executor.OutputCaptured(ctx, "nmcli", "-g", "ipv4.ignore-auto-dns", "connection", "show", conn)
	if err != nil {
		return "", "", fmt.Errorf("read ipv4.ignore-auto-dns for %s: %w", conn, err)
	}
	return strings.TrimSpace(string(dnsOut)), strings.TrimSpace(string(ignoreOut)), nil
}

// restoreConnDNS restores conn's captured DNS settings, normalising an empty
// ignoreAutoDNS to "no" so the revert never leaves it blank.
func restoreConnDNS(ctx context.Context, conn, dns, ignoreAutoDNS string) error {
	if ignoreAutoDNS == "" {
		ignoreAutoDNS = "no"
	}
	return executor.RunCaptured(ctx, "nmcli", "connection", "modify", conn, "ipv4.dns", dns, "ipv4.ignore-auto-dns", ignoreAutoDNS)
}

// RestoreSystemResolver undoes ConfigureSystemResolver by clearing the nmcli
// DNS override. Failures are logged but do not abort cleanup.
func RestoreSystemResolver(ctx context.Context, logger *slog.Logger) error {
	if isNetworkManagerActiveFn(ctx) {
		conn, err := hostnet.ActiveConnection(ctx)
		if err != nil {
			logger.Warn("resolver: could not detect active connection for restore", "err", err)
			return nil // best-effort restore; no active connection is non-fatal
		}

		logger.Info("resolver: restoring DHCP DNS", "conn", conn)

		modifyErr := hostnet.ClearConnectionDNSOverride(ctx, conn)
		if modifyErr != nil {
			logger.Warn("resolver: failed to clear DNS settings", "err", modifyErr)
		}

		upErr := hostnet.ActivateConnection(ctx, conn)
		if upErr != nil {
			logger.Warn("resolver: failed to apply DNS configuration", "err", upErr)
		}

		if modifyErr == nil && upErr == nil {
			logger.Info("resolver: system DNS restored to DHCP")
		}
	}

	return nil
}
