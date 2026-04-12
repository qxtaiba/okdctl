package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/packages"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// stopAndDisableService stops and disables a systemd service, logging warnings on failure.
func stopAndDisableService(ctx context.Context, serviceName string, logger *slog.Logger) {
	if system.IsServiceActive(ctx, serviceName) {
		if err := system.ManageService(ctx, system.ServiceStop, serviceName, serviceName+" service"); err != nil && logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to stop %s service: %v", serviceName, err))
		}
	} else if logger != nil {
		logger.Info(fmt.Sprintf("cleanup: %s service not running", serviceName))
	}

	if system.IsServiceEnabled(ctx, serviceName) {
		if err := system.ManageService(ctx, system.ServiceDisable, serviceName, serviceName+" service"); err != nil && logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to disable %s service: %v", serviceName, err))
		}
	} else if logger != nil {
		logger.Info(fmt.Sprintf("cleanup: %s service not enabled", serviceName))
	}
}

func removePackage(ctx context.Context, pkg string, logger *slog.Logger) {
	pm := detectPackageManager(logger)
	if err := packages.Remove(ctx, pm, []string{pkg}, logger); err != nil && logger != nil {
		logger.Warn(fmt.Sprintf("cleanup: failed to remove %s package: %v", pkg, err))
	}
}

func HAProxy(ctx context.Context, haproxyConfig, vip string, logger *slog.Logger) error {
	if logger != nil {
		logger.Info("cleanup: haproxy service and configuration")
	}

	stopAndDisableService(ctx, "haproxy", logger)

	_ = SafeRemoveWithLogger(ctx, haproxyConfig, "haproxy configuration file", logger)

	backupPattern := haproxyConfig + ".backup.*"
	backups, _ := filepath.Glob(backupPattern)
	for _, backup := range backups {
		_ = SafeRemoveWithLogger(ctx, backup, "haproxy backup configuration", logger)
	}

	if logger != nil {
		logger.Info("cleanup: removing okd firewall rules")
	}
	if err := firewall.RemoveOKDRules(ctx, true, logger); err != nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: firewall rules incomplete: %v", err))
		}
	} else if logger != nil {
		logger.Info("cleanup: firewall rules removed")
	}

	if vip != "" {
		iface, ifaceErr := netutil.GetDefaultInterface(ctx)
		if ifaceErr == nil {
			if rmErr := netutil.RemoveSecondaryIP(ctx, vip, iface); rmErr != nil {
				if logger != nil {
					logger.Warn(fmt.Sprintf("cleanup: could not remove vip %s from %s: %v", vip, iface, rmErr))
				}
			} else if logger != nil {
				logger.Info(fmt.Sprintf("cleanup: removed vip %s from %s", vip, iface))
			}
		}
	}

	removePackage(ctx, "haproxy", logger)

	if logger != nil {
		logger.Info("cleanup: haproxy completed")
	}

	return nil
}

func Apache(ctx context.Context, logger *slog.Logger) error {
	if logger != nil {
		logger.Info("cleanup: apache httpd service")
	}

	detectedOS := detectOS(logger)
	svcName := detectedOS.ApacheServiceName()
	pkgName := detectedOS.ApachePackageName()

	stopAndDisableService(ctx, svcName, logger)
	removePackage(ctx, pkgName, logger)

	if logger != nil {
		logger.Info("cleanup: apache httpd completed")
	}

	return nil
}

func WebServer(ctx context.Context, httpServerRoot string, logger *slog.Logger) error {
	ignitionDir := filepath.Join(httpServerRoot, "ignition")

	if _, err := os.Stat(ignitionDir); os.IsNotExist(err) {
		return nil
	}

	ignitionFiles, err := filepath.Glob(filepath.Join(ignitionDir, "*.ign"))
	if err != nil {
		return err
	}

	if len(ignitionFiles) == 0 {
		return nil
	}

	if logger != nil {
		logger.Info(fmt.Sprintf("cleanup: removing %d ignition files from web server", len(ignitionFiles)))
	}

	for _, f := range ignitionFiles {
		_ = SafeRemoveWithLogger(ctx, f, filepath.Base(f), nil)
	}

	return nil
}

func Dnsmasq(ctx context.Context, clusterName string, logger *slog.Logger) error {
	if logger != nil {
		logger.Info("cleanup: dnsmasq service and configuration")
	}

	if err := dns.RestoreSystemResolver(ctx, logger); err != nil && logger != nil {
		logger.Warn(fmt.Sprintf("cleanup: failed to restore system resolver: %v", err))
	}

	stopAndDisableService(ctx, "dnsmasq", logger)

	if clusterName != "" {
		if configPath, err := dns.DnsmasqConfigPath(fmt.Sprintf("okd-%s", clusterName)); err == nil {
			_ = system.RemoveAll(ctx, configPath, "dnsmasq okd config")
		} else if logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: invalid dnsmasq config name for cluster %q: %v", clusterName, err))
		}
	}

	configPattern := "/etc/dnsmasq.d/okd-*.conf"
	configs, _ := filepath.Glob(configPattern)
	for _, cfg := range configs {
		_ = system.RemoveAll(ctx, cfg, "dnsmasq okd config")
	}

	backupPattern := "/etc/dnsmasq.d/*.backup"
	backups, _ := filepath.Glob(backupPattern)
	for _, backup := range backups {
		_ = system.RemoveAll(ctx, backup, "dnsmasq backup config")
	}

	removePackage(ctx, "dnsmasq", logger)

	if logger != nil {
		logger.Info("cleanup: dnsmasq completed")
	}

	return nil
}
