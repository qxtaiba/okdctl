package cleanup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/dns"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/firewall"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/packages"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

func HAProxy(ctx context.Context, haproxyConfig, vip string, logger utils.Logger) error {
	if logger != nil {
		logger.Info("cleanup: haproxy service and configuration")
	}

	if system.IsServiceActive(ctx, "haproxy") {
		if err := system.ManageService(ctx, system.ServiceStop, "haproxy", "haproxy service"); err != nil && logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to stop haproxy service: %v", err))
		}
	} else if logger != nil {
		logger.Info("cleanup: haproxy service not running")
	}

	if system.IsServiceEnabled(ctx, "haproxy") {
		if err := system.ManageService(ctx, system.ServiceDisable, "haproxy", "haproxy service"); err != nil && logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to disable haproxy service: %v", err))
		}
	} else if logger != nil {
		logger.Info("cleanup: haproxy service not enabled")
	}

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

	if err := packages.Remove(ctx, []string{"haproxy"}, logger); err != nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to remove haproxy package: %v", err))
		}
	}

	if logger != nil {
		logger.Info("cleanup: haproxy completed")
	}

	return nil
}

func Apache(ctx context.Context, logger utils.Logger) error {
	if logger != nil {
		logger.Info("cleanup: apache httpd service")
	}

	if system.IsServiceActive(ctx, "httpd") {
		if err := system.ManageService(ctx, system.ServiceStop, "httpd", "httpd service"); err != nil && logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to stop httpd service: %v", err))
		}
	} else if logger != nil {
		logger.Info("cleanup: httpd service not running")
	}

	if system.IsServiceEnabled(ctx, "httpd") {
		if err := system.ManageService(ctx, system.ServiceDisable, "httpd", "httpd service"); err != nil && logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to disable httpd service: %v", err))
		}
	} else if logger != nil {
		logger.Info("cleanup: httpd service not enabled")
	}

	if err := packages.Remove(ctx, []string{"httpd"}, logger); err != nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to remove httpd package: %v", err))
		}
	}

	if logger != nil {
		logger.Info("cleanup: apache httpd completed")
	}

	return nil
}

func WebServer(ctx context.Context, httpServerRoot string, logger utils.Logger) error {
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

func Dnsmasq(ctx context.Context, clusterName string, logger utils.Logger) error {
	if logger != nil {
		logger.Info("cleanup: dnsmasq service and configuration")
	}

	if err := dns.RestoreSystemResolver(ctx, logger); err != nil && logger != nil {
		logger.Warn(fmt.Sprintf("cleanup: failed to restore system resolver: %v", err))
	}

	if system.IsServiceActive(ctx, "dnsmasq") {
		if err := system.ManageService(ctx, system.ServiceStop, "dnsmasq", "dnsmasq service"); err != nil && logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to stop dnsmasq service: %v", err))
		}
	}
	if system.IsServiceEnabled(ctx, "dnsmasq") {
		if err := system.ManageService(ctx, system.ServiceDisable, "dnsmasq", "dnsmasq service"); err != nil && logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to disable dnsmasq service: %v", err))
		}
	}

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

	if err := packages.Remove(ctx, []string{"dnsmasq"}, logger); err != nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to remove dnsmasq package: %v", err))
		}
	}

	if logger != nil {
		logger.Info("cleanup: dnsmasq completed")
	}

	return nil
}
