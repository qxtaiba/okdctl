package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
)

var criticalPaths = []string{"/", "/etc", "/var", "/usr", "/bin", "/sbin", "/lib", "/home", "/root", "/boot", "/dev", "/proc", "/sys"}

// refuseCriticalPath aborts if path resolves to a root-of-system location.
// Defense-in-depth against a config-file typo pointing cleanup at the wrong
// target. Returns nil for safe paths.
func refuseCriticalPath(path string) error {
	cleaned := filepath.Clean(path)
	for _, p := range criticalPaths {
		if cleaned == p {
			return fmt.Errorf("refusing to remove critical system path: %s", path)
		}
	}
	return nil
}

// SafeRemoveWithLogger removes a file or directory if it exists, logging
// failures via the provided logger. Returns nil if the path didn't exist or
// was removed successfully. Runs as root under the re-exec model so there
// is no fallback path to worry about.
func SafeRemoveWithLogger(_ context.Context, path, description string, logger *slog.Logger) error {
	if err := refuseCriticalPath(path); err != nil {
		if logger != nil {
			logger.Warn(err.Error())
		}
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := os.RemoveAll(path); err != nil {
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
