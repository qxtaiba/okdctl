package phase

import (
	"context"
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/hostnet"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// StopAndDisableService stops serviceName when active and disables it when
// enabled. Failures log at Warn and never abort — teardown must proceed past
// a wedged unit. What happens to the service's config afterwards diverges by
// caller on purpose: postinstall backs up haproxy.cfg before removing it,
// cleanup purges the config and every backup.
func StopAndDisableService(ctx context.Context, serviceName string, logger *slog.Logger) {
	logger = logutil.OrNop(logger)
	if system.IsServiceActive(ctx, serviceName) {
		if err := system.ManageService(ctx, system.ServiceStop, serviceName); err != nil {
			logger.Warn("failed to stop service", "svc", serviceName, "err", err)
		}
	} else {
		logger.Debug("service not running", "svc", serviceName)
	}

	if system.IsServiceEnabled(ctx, serviceName) {
		if err := system.ManageService(ctx, system.ServiceDisable, serviceName); err != nil {
			logger.Warn("failed to disable service", "svc", serviceName, "err", err)
		}
	} else {
		logger.Debug("service not enabled", "svc", serviceName)
	}
}

// ReleaseVIP removes vip from the host's default interface. No-op when vip
// is empty; detection and removal failures log at Warn because VIP release
// is best-effort during teardown.
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
