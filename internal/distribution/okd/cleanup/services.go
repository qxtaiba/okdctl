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

// dnsmasqConfPattern and dnsmasqBackupPattern are package-level vars so
// tests can redirect Dnsmasq's glob-and-remove loop to t.TempDir().
var (
	dnsmasqConfPattern   = "/etc/dnsmasq.d/okd-*.conf"
	dnsmasqBackupPattern = "/etc/dnsmasq.d/*.backup"
)

func removePackage(ctx context.Context, pkg string, logger *slog.Logger) {
	logger = logutil.OrNop(logger)
	pm := detectPackageManager(logger)
	if err := pm.Remove(ctx, []string{pkg}); err != nil {
		logger.Warn("cleanup: failed to remove package", "pkg", pkg, "err", err)
	}
}

// HAProxy stops the haproxy service, removes its config and backups,
// releases the VIP (when set), and uninstalls the haproxy package.
// Firewall rule removal is delegated to StepCleanupFirewall so destroy
// summary doesn't double-count the same operation.
//
// Assumes an okdctl-dedicated bastion: it deletes the live config plus every
// backup the glob matches, including setup's pristine pre-okdctl snapshot, so
// a pre-existing haproxy config is not restored.
func HAProxy(ctx context.Context, haproxyConfig, vip string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: haproxy service and configuration")

	phase.StopAndDisableService(ctx, "haproxy", logger)

	_ = SafeRemoveWithLogger(ctx, haproxyConfig, "haproxy configuration file", logger)

	// The glob covers postinstall's timestamped backups and setup's fixed
	// pristine snapshot; the latter used to be missed, leaving root-owned
	// residue in /etc/haproxy after uninstall.
	backups, _ := filepath.Glob(phase.HAProxyBackupGlob(haproxyConfig))
	for _, backup := range backups {
		_ = SafeRemoveWithLogger(ctx, backup, "haproxy backup configuration", logger)
	}

	phase.ReleaseVIP(ctx, vip, logger)

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

	phase.StopAndDisableService(ctx, svcName, logger)
	removePackage(ctx, pkgName, logger)

	logger.Info("cleanup: apache httpd completed")

	return nil
}

// WebServer removes generated *.ign files from the httpd ignition directory.
// Best-effort: per-file removal errors (including policy refusals from
// SafeRemoveWithLogger) are logged and swallowed, so a nil return does not
// guarantee the web root is clean — ignition payloads embedding the pull
// secret may still be served. It also returns nil early when the directory or
// glob is empty. Callers needing a hard guarantee must inspect the directory.
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

// Dnsmasq restores the system resolver, stops dnsmasq, removes the
// cluster-specific config, and uninstalls the dnsmasq package.
//
// Assumes an okdctl-dedicated bastion: after the cluster's own okd-<name>.conf
// is removed it also purges every /etc/dnsmasq.d/okd-*.conf and every
// *.backup regardless of origin, so a second cluster's DNS config or a foreign
// backup on a shared bastion would be destroyed too.
func Dnsmasq(ctx context.Context, clusterName string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	logger.Info("cleanup: dnsmasq service and configuration")

	if err := dns.RestoreSystemResolver(ctx, logger); err != nil {
		logger.Warn("cleanup: failed to restore system resolver", "err", err)
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
