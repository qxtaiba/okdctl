package phase

import (
	"context"
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/hostnet"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// StopAndDisableService stops and disables serviceName if active/enabled,
// logging failures at Warn without aborting so teardown survives a wedged
// unit; callers own the config file afterwards (postinstall backs it up,
// cleanup purges it).
func StopAndDisableService(ctx context.Context, serviceName string, logger *slog.Logger) {
	logger = logutil.OrNop(logger)
	if system.IsServiceActive(ctx, serviceName) {
		if err := system.ManageService(ctx, system.ServiceStop, serviceName); err != nil {
			logger.Warn("could not stop service", "svc", serviceName, "err", err)
		}
	} else {
		logger.Debug("service not running", "svc", serviceName)
	}

	if system.IsServiceEnabled(ctx, serviceName) {
		if err := system.ManageService(ctx, system.ServiceDisable, serviceName); err != nil {
			logger.Warn("could not disable service", "svc", serviceName, "err", err)
		}
	} else {
		logger.Debug("service not enabled", "svc", serviceName)
	}
}

// ReleaseVIP removes vip from the host's default interface; a no-op if vip
// is empty, and detection/removal failures log at Warn (best-effort).
func ReleaseVIP(ctx context.Context, vip string, logger *slog.Logger) {
	if vip == "" {
		return
	}
	logger = logutil.OrNop(logger)
	iface, err := hostnet.GetDefaultInterface(ctx)
	if err != nil {
		logger.Warn("could not detect default interface for vip removal", "vip", vip, "err", err)
		return
	}
	if err := hostnet.RemoveSecondaryIP(ctx, vip, iface); err != nil {
		logger.Warn("could not remove vip", "vip", vip, "iface", iface, "err", err)
		return
	}
	logger.Info("removed vip", "vip", vip, "iface", iface)
}
