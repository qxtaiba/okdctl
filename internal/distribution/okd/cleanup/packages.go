package cleanup

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/platform"
)

// detectPackageManager returns a Manager for the current host OS, falling
// back to RHEL/dnf if detection fails (cleanup runs on the bastion).
func detectPackageManager(logger *slog.Logger) *platform.Manager {
	return platform.NewPackageManager(platform.DetectOrDefault(logger), logger)
}

// InstalledPackages returns the scoped list of dnf packages cleanup will
// uninstall. Exported so a future cleanup preview/plan CLI verb can render
// this list without executing the removal.
func InstalledPackages() []string {
	return []string{
		"coreos-installer",
		"terraform",
	}
}

// InstalledBinaries returns the installer-managed binaries cleanup will
// remove from BinDir. Exported so a future cleanup preview/plan CLI verb
// can render this list without executing the removal.
func InstalledBinaries() []string {
	okdBinaries := []string{
		"openshift-install",
		"oc",
		"kubectl",
	}
	return append(okdBinaries, phase.ExternalToolBinaries()...)
}

// Packages removes dnf packages and tool binaries installed during setup.
// Individual failures are logged and aggregated; the function returns an
// error only if at least one removal failed. Empty binDir falls back to
// config.DefaultBinDir.
func Packages(ctx context.Context, binDir string, logger *slog.Logger) error {
	binDir = config.BinDirOrDefault(binDir)
	logger = logutil.OrNop(logger)
	if err := refuseCriticalPath(binDir); err != nil {
		return &errtypes.ClusterError{Msg: "refusing binary removal from critical path"}
	}
	var hasErrors bool

	pm := detectPackageManager(logger)

	pkgList := InstalledPackages()
	if err := pm.Remove(ctx, pkgList); err != nil {
		logger.Warn("cleanup: some packages could not be removed (may require manual cleanup)")
		hasErrors = true
	}

	binaries := InstalledBinaries()
	for _, binary := range binaries {
		binPath := filepath.Join(binDir, binary)
		if _, err := os.Stat(binPath); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if guardErr := refuseCriticalPath(binPath); guardErr != nil {
			logger.Warn("cleanup: refusing critical path", "err", guardErr)
			hasErrors = true
			continue
		}

		if err := os.RemoveAll(binPath); err != nil {
			logger.Warn("cleanup: failed to remove binary", "path", binPath, "err", err)
			hasErrors = true
		} else {
			logger.Info("cleanup: removed", "path", binPath)
		}
	}

	if hasErrors {
		return &errtypes.ClusterError{Msg: "some cleanup operations failed"}
	}
	return nil
}
