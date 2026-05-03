package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

var criticalPaths = []string{"/", "/etc", "/var", "/usr", "/usr/local", "/bin", "/sbin", "/lib", "/home", "/root", "/boot", "/dev", "/proc", "/sys"}

// refuseCriticalPath aborts if path resolves to a root-of-system location.
// Defense-in-depth against a config-file typo pointing cleanup at the wrong
// target. Returns nil for safe paths.
func refuseCriticalPath(path string) error {
	if slices.Contains(criticalPaths, filepath.Clean(path)) {
		return fmt.Errorf("refusing to remove critical system path: %s", path)
	}
	return nil
}

// SafeRemoveWithLogger removes a file or directory if it exists, logging
// failures via the provided logger. Returns nil if the path didn't exist or
// was removed successfully. Runs as root under the re-exec model so there
// is no fallback path to worry about.
func SafeRemoveWithLogger(ctx context.Context, path, description string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	if err := refuseCriticalPath(path); err != nil {
		logger.Warn("cleanup: refusing critical path", "err", err)
		return &errtypes.ConfigError{Msg: "cleanup refused critical path", Err: err}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		logger.Warn("cleanup: could not remove", "target", description, "err", err)
		return &errtypes.ConfigError{Msg: fmt.Sprintf("could not remove %s", description), Err: err}
	}

	if _, err := os.Stat(path); err == nil {
		logger.Warn("cleanup: still exists after removal", "target", description)
		return &errtypes.ConfigError{Msg: fmt.Sprintf("%s still exists after removal", description)}
	} else if !os.IsNotExist(err) {
		// Cannot verify removal (e.g. permission denied on parent); assume success.
		logger.Warn("cleanup: could not verify removal", "target", description, "err", err)
	}

	return nil
}

// WorkDirectory removes all generated artifacts under workDir. When
// preserveConfig is true the okdctl.yaml at the root is kept; everything
// else is removed best-effort and the first failure is returned.
func WorkDirectory(ctx context.Context, workDir string, preserveConfig bool, logger *slog.Logger) error {
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return nil
	}

	logger = logutil.OrNop(logger)
	logger.Info("cleanup: removing work directory")

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
