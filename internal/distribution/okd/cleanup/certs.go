package cleanup

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/platform"
)

// IgnitionCerts removes the generated ignition TLS cert/key directory and the
// apache HTTPS vhost drop-in conf; setup regenerates the cert if missing, so
// removal is safe between runs.
func IgnitionCerts(ctx context.Context, projectRoot string, logger *slog.Logger) error {
	certDir := filepath.Join(projectRoot, "certs", "ignition")
	osInfo := platform.DetectOrDefault(logger)
	confPath := filepath.Join(osInfo.ApacheVhostConfDir(), "ignition-ssl.conf")

	errs := []error{
		SafeRemoveWithLogger(ctx, certDir, "ignition TLS certs", logger),
		SafeRemoveWithLogger(ctx, confPath, "ignition-ssl.conf vhost drop-in", logger),
	}
	return errors.Join(errs...)
}
