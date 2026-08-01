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

// dnsmasqConfigDir is the directory for per-cluster dnsmasq fragments.
// Tests override this var to redirect writes to a t.TempDir().
var dnsmasqConfigDir = phase.DefaultDNSMasqConfigDir

// resolvedConf is the systemd-resolved drop-in written by ConfigureSystemResolver.
// Tests override this var to redirect operations to a t.TempDir().
var resolvedConf = "/etc/systemd/resolved.conf.d/dnsmasq.conf"

var (
	// validateDnsmasqConfigFn and restartDnsmasqFn are package-level vars so
	// tests can inject fakes without a real dnsmasq binary on PATH.
	validateDnsmasqConfigFn = ValidateDnsmasqConfig
	restartDnsmasqFn        = RestartDnsmasq
	// removeAllFn is the os.RemoveAll indirection used by RestoreSystemResolver.
	// Tests inject a failing func to cover the logged-but-not-propagated error path.
	removeAllFn = os.RemoveAll
	// isNetworkManagerActiveFn and isServiceActiveFn let tests drive the
	// resolver forward/restore paths from non-Linux hosts, where the real
	// probes are hard-gated off by runtime.GOOS.
	isNetworkManagerActiveFn = IsNetworkManagerActive
	isServiceActiveFn        = system.IsServiceActive
)

var validConfigNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// EnableDnsmasq marks dnsmasq to start at boot (systemctl enable, no
// --now): it does not start the service — the restart after the first
// config deploy brings it up.
func EnableDnsmasq(ctx context.Context) error {
	return system.ManageService(ctx, system.ServiceEnable, dnsmasqService)
}

// RestartDnsmasq restarts dnsmasq so a newly written config takes effect.
// Run ValidateDnsmasqConfig first — restarting into a broken config takes
// cluster DNS down.
func RestartDnsmasq(ctx context.Context) error {
	return system.ManageService(ctx, system.ServiceRestart, dnsmasqService)
}

// ValidateDnsmasqConfig runs "dnsmasq --test" to verify the on-disk config.
// stderr is captured so the returned error carries dnsmasq's actual
// syntax-error message, not just "exit status 1".
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
			return fmt.Errorf("back up config %s: %w", configPath, err)
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := system.AtomicWriteString(configPath, content, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", configPath, err)
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

	if isNetworkManagerActiveFn(ctx) {
		conn, err := hostnet.ActiveConnection(ctx)
		if err != nil {
			return err
		}

		// Capture the pre-change profile before any mutation so a failed
		// connection-up can revert it. Fail before touching the profile if the
		// capture itself fails — a half-applied override with no way back is
		// worse than not starting.
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
			// The persistent profile now forces 127.0.0.1; connection-up failed
			// so dnsmasq may not be reachable. Revert the profile so the host
			// does not resurrect a dead resolver on the next reconnect/reboot.
			// Detached ctx: a Ctrl-C that killed the up must not doom the revert.
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
		confDir := filepath.Dir(resolvedConf)
		confContent := "[Resolve]\nDNS=127.0.0.1\nDomains=~.\n"
		if err := os.MkdirAll(confDir, 0o755); err != nil {
			return fmt.Errorf("create resolved.conf.d: %w", err)
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
			return fmt.Errorf("write dnsmasq.conf: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		defer func() { _ = os.Remove(tmpPath) }()
		if err := system.CopyFile(tmpPath, confPath); err != nil {
			return fmt.Errorf("install dnsmasq.conf: %w", err)
		}
		if err := executor.RunCaptured(ctx, "systemctl", "restart", "systemd-resolved"); err != nil {
			// The drop-in now forces DNS=127.0.0.1; resolved failed to restart,
			// so remove it to let the host fall back to its prior resolver
			// rather than a dead one, then re-restart to converge. Detached ctx
			// for the compensating restart, matching the revert above.
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

// resolverRestoreTimeout bounds the detached compensating call issued when a
// resolver mutation partially applied and must be rolled back.
const resolverRestoreTimeout = 30 * time.Second

// captureConnDNS reads the current ipv4.dns and ipv4.ignore-auto-dns of an
// nmcli connection so ConfigureSystemResolver can revert them if the follow-up
// connection-up fails.
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

// restoreConnDNS rewrites conn's DNS settings back to the captured values. An
// empty ignoreAutoDNS is normalised to "no" (the NetworkManager default) so the
// revert never leaves the field blank.
func restoreConnDNS(ctx context.Context, conn, dns, ignoreAutoDNS string) error {
	if ignoreAutoDNS == "" {
		ignoreAutoDNS = "no"
	}
	return executor.RunCaptured(ctx, "nmcli", "connection", "modify", conn, "ipv4.dns", dns, "ipv4.ignore-auto-dns", ignoreAutoDNS)
}

// RestoreSystemResolver undoes ConfigureSystemResolver: it clears the
// nmcli DNS override or removes the systemd-resolved drop-in. Failures are
// logged but do not abort cleanup.
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
