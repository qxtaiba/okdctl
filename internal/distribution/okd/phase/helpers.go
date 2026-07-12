package phase

import (
	"fmt"
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/netutil"
)

// WarnOnError returns an OnError callback that logs a warning with the given
// message prefix followed by the error.
func WarnOnError(logger *slog.Logger, msg string) func(error) {
	return func(err error) {
		logger.Warn(msg, "err", err)
	}
}

// ResolveClusterVIP resolves the kube-vip address from config: either the
// explicit Networking.Bastion.VIP value or the .10 derivation from
// Networking.StaticIP.Start.
func ResolveClusterVIP(cfg *config.Config) (string, error) {
	vip, err := netutil.ResolveVIP(cfg.Networking.Bastion.VIP, cfg.Networking.StaticIP.Start)
	if err != nil {
		return "", fmt.Errorf("resolve VIP: %w", err)
	}
	return vip, nil
}
