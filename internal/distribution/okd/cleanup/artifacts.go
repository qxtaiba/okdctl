package cleanup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/paths"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// SafeRemoveWithLogger removes a file or directory if it exists, with automatic sudo fallback.
// Returns nil if the path doesn't exist or was successfully removed.
// This is a best-effort operation - errors are logged but nil is returned for cleanup scenarios
// where partial success is acceptable.
// Note: If elevated privileges are required, sudo may prompt for a password.
func SafeRemoveWithLogger(ctx context.Context, path, description string, logger logging.Logger) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Silently skip non-existent paths
	}

	if err := system.RemoveAll(ctx, path, description); err != nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("could not remove %s: %v", description, err))
		}
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		if logger != nil {
			logger.Warn(fmt.Sprintf("%s still exists after removal", description))
		}
	}

	return nil
}

// WorkDirectory removes working directory artifacts.
func WorkDirectory(ctx context.Context, workDir string, preserveConfig bool, logger logging.Logger) error {
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return nil
	}

	if logger != nil {
		logger.Info("cleanup: removing work directory")
	}

	if preserveConfig {
		_ = SafeRemoveWithLogger(ctx, filepath.Join(workDir, "tmp"), "temporary files", logger)
		_ = SafeRemoveWithLogger(ctx, filepath.Join(workDir, "downloads"), "download cache", logger)
		_ = SafeRemoveWithLogger(ctx, filepath.Join(workDir, "installer"), "installer files", logger)
		_ = SafeRemoveWithLogger(ctx, filepath.Join(workDir, "custom-isos"), "custom ISO files", logger)
	} else {
		_ = SafeRemoveWithLogger(ctx, paths.ClusterConfigDir(workDir), "cluster configuration", logger)
		_ = SafeRemoveWithLogger(ctx, filepath.Join(workDir, "custom-isos"), "custom ISOs", logger)
		_ = SafeRemoveWithLogger(ctx, filepath.Join(workDir, "installer"), "installer", logger)
		_ = SafeRemoveWithLogger(ctx, filepath.Join(workDir, "tmp"), "temp files", logger)
		_ = SafeRemoveWithLogger(ctx, filepath.Join(workDir, "downloads"), "downloads", logger)
		_ = SafeRemoveWithLogger(ctx, workDir, "work directory", logger)
	}

	return nil
}
