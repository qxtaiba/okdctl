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

// resolvedConf is the systemd-resolved drop-in path; overridden to t.TempDir() in tests.
var resolvedConf = "/etc/systemd/resolved.conf.d/dnsmasq.conf"

var (
	// validateDnsmasqConfigFn/restartDnsmasqFn: package vars so tests can
	// inject fakes without a real dnsmasq binary.
	validateDnsmasqConfigFn = ValidateDnsmasqConfig
	restartDnsmasqFn        = RestartDnsmasq
	// removeAllFn: os.RemoveAll indirection for injecting a failing func in
	// RestoreSystemResolver tests.
	removeAllFn = os.RemoveAll
	// isNetworkManagerActiveFn/isServiceActiveFn let tests drive resolver paths
	// on non-Linux hosts, bypassing the runtime.GOOS gate.
	isNetworkManagerActiveFn = IsNetworkManagerActive
	isServiceActiveFn        = system.IsServiceActive
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

// ConfigureSystemResolver points system DNS at localhost (dnsmasq), using
// fallbackDNS for queries dnsmasq can't resolve. It tries NetworkManager then
// systemd-resolved, warning if neither is found.
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

	if isServiceActiveFn(ctx, "systemd-resolved") {
		logger.Info("resolver: configuring systemd-resolved to use dnsmasq")
		confPath := resolvedConf
		if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
			return fmt.Errorf("create resolved.conf.d: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		tmpPath, err := system.WriteTempFile("resolved-conf", 0o644, func(f *os.File) error {
			_, err := f.WriteString("[Resolve]\nDNS=127.0.0.1\nDomains=~.\n")
			return err
		})
		if err != nil {
			return fmt.Errorf("write dnsmasq.conf: %w", err)
		}
		defer func() { _ = os.Remove(tmpPath) }()
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := system.CopyFile(tmpPath, confPath); err != nil {
			return fmt.Errorf("install dnsmasq.conf: %w", err)
		}
		if err := executor.RunCaptured(ctx, "systemctl", "restart", "systemd-resolved"); err != nil {
			// resolved restart failed with DNS forced to 127.0.0.1; remove the
			// drop-in and re-restart (detached ctx) so the host falls back
			// instead of staying dead.
			if rmErr := removeAllFn(confPath); rmErr != nil {
				return fmt.Errorf("restart systemd-resolved: %w (drop-in %s left in place; removing it also failed: %w)", err, confPath, rmErr)
			}
			rCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resolverRestoreTimeout)
			defer cancel()
			_ = executor.RunCaptured(rCtx, "systemctl", "restart", "systemd-resolved")
			return fmt.Errorf("restart systemd-resolved: %w (drop-in %s removed)", err, confPath)
		}
		return nil
	}

	logger.Warn("resolver: neither NetworkManager nor systemd-resolved found, skipping system resolver configuration")
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

// RestoreSystemResolver undoes ConfigureSystemResolver: clears the nmcli DNS
// override or removes the systemd-resolved drop-in. Failures are logged but do
// not abort cleanup.
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
		return nil
	}

	if system.FileExists(resolvedConf) {
		logger.Info("resolver: removing systemd-resolved dnsmasq configuration")
		if err := removeAllFn(resolvedConf); err != nil {
			logger.Warn("resolver: failed to remove", "path", resolvedConf, "err", err)
			return nil
		}
		if isServiceActiveFn(ctx, "systemd-resolved") {
			_ = system.ManageService(ctx, system.ServiceRestart, "systemd-resolved")
		}
		logger.Info("resolver: systemd-resolved configuration restored")
	}

	return nil
}
