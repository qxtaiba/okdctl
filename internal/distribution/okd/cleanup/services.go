package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/platform"
)

// dnsmasqConfPattern/dnsmasqBackupPattern are vars so tests can redirect the glob-and-remove loop.
var (
	dnsmasqConfPattern   = "/etc/dnsmasq.d/okd-*.conf"
	dnsmasqBackupPattern = "/etc/dnsmasq.d/*.backup"
)

func removePackage(ctx context.Context, pkg string, logger *slog.Logger) {
	logger = logutil.OrNop(logger)
	pm := detectPackageManager(logger)
	if err := pm.Remove(ctx, []string{pkg}); err != nil {
		logger.Warn("cleanup: could not remove package", "pkg", pkg, "err", err)
	}
}

// HAProxy stops the haproxy service, removes its config, backups, and VIP,
// and uninstalls the package. Assumes a dedicated bastion: it deletes every
// backup the glob matches.
func HAProxy(ctx context.Context, haproxyConfig, vip string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: haproxy service and configuration")

	phase.StopAndDisableService(ctx, "haproxy", logger)

	_ = SafeRemoveWithLogger(ctx, haproxyConfig, "haproxy configuration file", logger)

	// Covers both postinstall's timestamped backups and setup's fixed pristine snapshot.
	backups, _ := filepath.Glob(phase.HAProxyBackupGlob(haproxyConfig))
	for _, backup := range backups {
		_ = SafeRemoveWithLogger(ctx, backup, "haproxy backup configuration", logger)
	}

	phase.ReleaseVIP(ctx, vip, logger)

	removePackage(ctx, "haproxy", logger)

	logger.Info("cleanup: haproxy completed")

	return nil
}

// Apache stops the httpd service and removes the apache package using platform-appropriate names.
func Apache(ctx context.Context, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: apache httpd service")

	detectedOS := platform.DetectOrDefault(logger)
	svcName := detectedOS.ApacheServiceName()
	pkgName := detectedOS.ApachePackageName()

	phase.StopAndDisableService(ctx, svcName, logger)
	removePackage(ctx, pkgName, logger)

	logger.Info("cleanup: apache httpd completed")

	return nil
}

// WebServer removes generated *.ign files from the httpd ignition directory, best-effort.
// A nil return does not guarantee the web root is clean — ignition payloads
// embedding the pull secret may still be served.
func WebServer(ctx context.Context, httpServerRoot string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	ignitionDir := filepath.Join(httpServerRoot, "ignition")

	if _, err := os.Stat(ignitionDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	ignitionFiles, err := filepath.Glob(filepath.Join(ignitionDir, "*.ign"))
	if err != nil {
		return &errtypes.ConfigError{Msg: "glob ignition files", Err: err}
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

// Dnsmasq restores the system resolver, stops dnsmasq, removes the cluster
// config, and uninstalls the package. Assumes a dedicated bastion: it purges
// every okd-*.conf and *.backup regardless of origin.
func Dnsmasq(ctx context.Context, clusterName string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: dnsmasq service and configuration")

	if err := dns.RestoreSystemResolver(ctx, logger); err != nil {
		logger.Warn("cleanup: could not restore system resolver", "err", err)
	}

	phase.StopAndDisableService(ctx, "dnsmasq", logger)

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

	removeGlobbed := func(pattern string) {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if guardErr := refuseCriticalPath(m); guardErr != nil {
				logger.Warn("cleanup: refusing critical path", "path", m, "err", guardErr)
				continue
			}
			_ = os.RemoveAll(m)
		}
	}
	removeGlobbed(dnsmasqConfPattern)
	removeGlobbed(dnsmasqBackupPattern)

	removePackage(ctx, "dnsmasq", logger)

	logger.Info("cleanup: dnsmasq completed")

	return nil
}
