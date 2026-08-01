package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/workspace"
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
//
// Refuses critical system paths and symlinks, returning a ConfigError with the
// path left untouched; callers must not read a non-nil return as an OS-level
// failure — it can mean the removal was declined by policy.
func SafeRemoveWithLogger(ctx context.Context, path, description string, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	if err := refuseCriticalPath(path); err != nil {
		logger.Warn("cleanup: refusing critical path", "err", err)
		return &errtypes.ConfigError{Msg: "cleanup refused critical path", Err: err}
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			logger.Warn("cleanup: refusing symlink target; remove the link manually", "path", path)
			return &errtypes.ConfigError{Msg: fmt.Sprintf("refusing to remove symlink %s", path)}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		logger.Warn("cleanup: could not remove", "target", description, "path", path, "err", err)
		return &errtypes.ConfigError{Msg: fmt.Sprintf("could not remove %s", description), Err: err}
	}

	if _, err := os.Stat(path); err == nil {
		logger.Warn("cleanup: still exists after removal", "target", description, "path", path)
		return &errtypes.ConfigError{Msg: fmt.Sprintf("%s still exists after removal", description)}
	} else if !errors.Is(err, os.ErrNotExist) {
		// Cannot verify removal (e.g. permission denied on parent); assume success.
		logger.Warn("cleanup: could not verify removal", "target", description, "path", path, "err", err)
	}

	return nil
}

// WorkDirectory removes all generated artifacts under workDir, including
// workDir itself. Best-effort: failures accumulate into the returned joined
// error rather than aborting early.
//
// retainClusterConfig preserves cluster-config (auth/kubeconfig and
// auth/kubeadmin-password — the only admin credentials) and the workDir
// root that contains it; everything else is still removed. Callers set it
// when terraform state says the cluster is still live.
//
// All Full-sequence steps that follow this one (webServer, haproxy,
// dnsmasq, terraform, packages, ignition-certs) reference paths outside
// workDir, so a partial-strip from a mid-run crash does not break
// subsequent cleanup steps.
func WorkDirectory(ctx context.Context, workDir string, retainClusterConfig bool, logger *slog.Logger) error {
	if _, err := os.Stat(workDir); errors.Is(err, os.ErrNotExist) {
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

	if !retainClusterConfig {
		remove(workspace.ClusterConfigDir(workDir), "cluster configuration")
	}
	remove(filepath.Join(workDir, "custom-isos"), "custom ISOs")
	remove(filepath.Join(workDir, "installer"), "installer")
	remove(filepath.Join(workDir, "tmp"), "temp files")
	remove(filepath.Join(workDir, "downloads"), "downloads")
	if !retainClusterConfig {
		remove(workDir, "work directory")
	}

	return errors.Join(errs...)
}
