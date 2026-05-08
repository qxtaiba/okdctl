package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/platform"
	"github.com/qxtaiba/okdctl/internal/system"
)

// dnsmasqConfPattern and dnsmasqBackupPattern are package-level vars so
// tests can redirect Dnsmasq's glob-and-remove loop to t.TempDir().
var (
	dnsmasqConfPattern   = "/etc/dnsmasq.d/okd-*.conf"
	dnsmasqBackupPattern = "/etc/dnsmasq.d/*.backup"
)

func stopAndDisableService(ctx context.Context, serviceName string, logger *slog.Logger) {
	logger = logutil.OrNop(logger)
	if system.IsServiceActive(ctx, serviceName) {
		if err := system.ManageService(ctx, system.ServiceStop, serviceName, serviceName+" service"); err != nil {
			logger.Warn("cleanup: failed to stop service", "svc", serviceName, "err", err)
		}
	} else {
		logger.Info("cleanup: service not running", "svc", serviceName)
	}

	if system.IsServiceEnabled(ctx, serviceName) {
		if err := system.ManageService(ctx, system.ServiceDisable, serviceName, serviceName+" service"); err != nil {
			logger.Warn("cleanup: failed to disable service", "svc", serviceName, "err", err)
		}
	} else {
		logger.Info("cleanup: service not enabled", "svc", serviceName)
	}
}

func removePackage(ctx context.Context, pkg string, logger *slog.Logger) {
	logger = logutil.OrNop(logger)
	pm := detectPackageManager(logger)
	if err := pm.Remove(ctx, []string{pkg}, logger); err != nil {
		logger.Warn("cleanup: failed to remove package", "pkg", pkg, "err", err)
	}
}

// HAProxy stops the haproxy service, removes its config and backups, tears
// down okdctl-managed firewall rules, releases the VIP (when set), and
// uninstalls the haproxy package.
func HAProxy(ctx context.Context, haproxyConfig, vip string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: haproxy service and configuration")

	stopAndDisableService(ctx, "haproxy", logger)

	_ = SafeRemoveWithLogger(ctx, haproxyConfig, "haproxy configuration file", logger)

	backupPattern := haproxyConfig + ".backup.*"
	backups, _ := filepath.Glob(backupPattern)
	for _, backup := range backups {
		_ = SafeRemoveWithLogger(ctx, backup, "haproxy backup configuration", logger)
	}

	logger.Info("cleanup: removing okd firewall rules")
	if err := firewall.RemoveOKDRules(ctx, true, logger); err != nil {
		logger.Warn("cleanup: firewall rules incomplete", "err", err)
	} else {
		logger.Info("cleanup: firewall rules removed")
	}

	if vip != "" {
		iface, ifaceErr := netutil.GetDefaultInterface(ctx)
		if ifaceErr == nil {
			if rmErr := netutil.RemoveSecondaryIP(ctx, vip, iface); rmErr != nil {
				logger.Warn("cleanup: could not remove vip", "vip", vip, "iface", iface, "err", rmErr)
			} else {
				logger.Info("cleanup: removed vip", "vip", vip, "iface", iface)
			}
		}
	}

	removePackage(ctx, "haproxy", logger)

	logger.Info("cleanup: haproxy completed")

	return nil
}

// Apache stops the httpd service and removes the apache package using the
// platform-appropriate service and package names.
func Apache(ctx context.Context, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: apache httpd service")

	detectedOS := platform.DetectOrDefault(logger)
	svcName := detectedOS.ApacheServiceName()
	pkgName := detectedOS.ApachePackageName()

	stopAndDisableService(ctx, svcName, logger)
	removePackage(ctx, pkgName, logger)

	logger.Info("cleanup: apache httpd completed")

	return nil
}

// WebServer removes generated *.ign files from the httpd ignition directory.
func WebServer(ctx context.Context, httpServerRoot string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	ignitionDir := filepath.Join(httpServerRoot, "ignition")

	if _, err := os.Stat(ignitionDir); os.IsNotExist(err) {
		return nil
	}

	ignitionFiles, err := filepath.Glob(filepath.Join(ignitionDir, "*.ign"))
	if err != nil {
		return &errtypes.ConfigError{Msg: "failed to glob ignition files", Err: err}
	}

	if len(ignitionFiles) == 0 {
		return nil
	}

	logger.Info("cleanup: removing ignition files from web server", "count", len(ignitionFiles))

	for _, f := range ignitionFiles {
		_ = SafeRemoveWithLogger(ctx, f, filepath.Base(f), logger)
	}

	return nil
}

// Dnsmasq restores the system resolver, stops dnsmasq, removes the
// cluster-specific config, and uninstalls the dnsmasq package.
func Dnsmasq(ctx context.Context, clusterName string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: dnsmasq service and configuration")

	if err := dns.RestoreSystemResolver(ctx, logger); err != nil {
		logger.Warn("cleanup: failed to restore system resolver", "err", err)
	}

	stopAndDisableService(ctx, "dnsmasq", logger)

	if clusterName != "" {
		if configPath, err := dns.DnsmasqConfigPath(fmt.Sprintf("okd-%s", clusterName)); err == nil {
			if guardErr := refuseCriticalPath(configPath); guardErr != nil {
				logger.Warn("cleanup: refusing critical path", "path", configPath, "err", guardErr)
			} else {
				_ = os.RemoveAll(configPath)
			}
		} else {
			logger.Warn("cleanup: invalid dnsmasq config name for cluster", "cluster", clusterName, "err", err)
		}
	}

	configs, _ := filepath.Glob(dnsmasqConfPattern)
	for _, cfg := range configs {
		if guardErr := refuseCriticalPath(cfg); guardErr != nil {
			logger.Warn("cleanup: refusing critical path", "path", cfg, "err", guardErr)
			continue
		}
		_ = os.RemoveAll(cfg)
	}

	backups, _ := filepath.Glob(dnsmasqBackupPattern)
	for _, backup := range backups {
		if guardErr := refuseCriticalPath(backup); guardErr != nil {
			logger.Warn("cleanup: refusing critical path", "path", backup, "err", guardErr)
			continue
		}
		_ = os.RemoveAll(backup)
	}

	removePackage(ctx, "dnsmasq", logger)

	logger.Info("cleanup: dnsmasq completed")

	return nil
}
