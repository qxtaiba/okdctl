package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// SafeRemoveWithLogger removes a file or directory if it exists, with automatic sudo fallback.
// Returns nil if the path doesn't exist or was successfully removed, or a wrapped error on failure.
// Errors are also logged via the provided logger for visibility in best-effort cleanup phase.
// Note: If elevated privileges are required, sudo may prompt for a password.
func SafeRemoveWithLogger(ctx context.Context, path, description string, logger *slog.Logger) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Silently skip non-existent paths
	}

	if err := system.RemoveAll(ctx, path, description); err != nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("could not remove %s: %v", description, err))
		}
		return fmt.Errorf("could not remove %s: %w", description, err)
	}

	if _, err := os.Stat(path); err == nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("%s still exists after removal", description))
		}
		return fmt.Errorf("%s still exists after removal", description)
	} else if !os.IsNotExist(err) {
		// Cannot verify removal (e.g. permission denied on parent); assume success.
		if logger != nil {
			logger.Warn(fmt.Sprintf("could not verify removal of %s: %v", description, err))
		}
	}

	return nil
}

func WorkDirectory(ctx context.Context, workDir string, preserveConfig bool, logger *slog.Logger) error {
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return nil
	}

	if logger != nil {
		logger.Info("cleanup: removing work directory")
	}

	var errs []error
	remove := func(path, description string) {
		if err := SafeRemoveWithLogger(ctx, path, description, logger); err != nil {
			errs = append(errs, err)
		}
	}

	if preserveConfig {
		remove(filepath.Join(workDir, "tmp"), "temporary files")
		remove(filepath.Join(workDir, "downloads"), "download cache")
		remove(filepath.Join(workDir, "installer"), "installer files")
		remove(filepath.Join(workDir, "custom-isos"), "custom ISO files")
	} else {
		remove(phase.ClusterConfigDir(workDir), "cluster configuration")
		remove(filepath.Join(workDir, "custom-isos"), "custom ISOs")
		remove(filepath.Join(workDir, "installer"), "installer")
		remove(filepath.Join(workDir, "tmp"), "temp files")
		remove(filepath.Join(workDir, "downloads"), "downloads")
		remove(workDir, "work directory")
	}

	return errors.Join(errs...)
}
