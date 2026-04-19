// Package packages wraps system package management for OKD host
// dependencies (haproxy, dnsmasq, httpd, and related tooling).
package packages

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/platform"
)

// Install installs the given packages via pm and logs a grouped status line
// using description. Empty slices are a no-op.
func Install(ctx context.Context, pm platform.PackageManager, packages []string, description string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}

	logger.Info(fmt.Sprintf("packages: installing %s", description))
	if err := pm.Install(ctx, packages, logger); err != nil {
		return fmt.Errorf("install %s failed: %w", description, err)
	}

	logger.Info(fmt.Sprintf("packages: %s installed", description))
	return nil
}

// Remove uninstalls the given packages via pm. Empty slices are a no-op.
func Remove(ctx context.Context, pm platform.PackageManager, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}

	logger.Info(fmt.Sprintf("packages: removing %d package(s)", len(packages)))
	if err := pm.Remove(ctx, packages, logger); err != nil {
		return fmt.Errorf("package removal failed: %w", err)
	}

	logger.Info("packages: removed successfully")
	return nil
}
