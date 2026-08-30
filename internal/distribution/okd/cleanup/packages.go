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

// detectPackageManager returns a Manager for the host OS, falling back to
// RHEL/dnf on detection failure.
func detectPackageManager(logger *slog.Logger) *platform.Manager {
	return platform.NewPackageManager(platform.DetectOrDefault(logger), logger)
}

// InstalledPackages returns the dnf packages cleanup will uninstall.
func InstalledPackages() []string {
	return []string{
		"coreos-installer",
		"terraform",
	}
}

// InstalledBinaries returns the installer-managed binaries cleanup removes from BinDir.
func InstalledBinaries() []string {
	return append(phase.OKDToolBinaries(), phase.ExternalToolBinaries()...)
}

// Packages removes dnf packages and tool binaries installed during setup,
// aggregating failures into a single error; empty binDir defaults to config.DefaultBinDir.
func Packages(ctx context.Context, binDir string, logger *slog.Logger) error {
	binDir = config.BinDirOrDefault(binDir)
	logger = logutil.OrNop(logger)
	if err := refuseCriticalPath(binDir); err != nil {
		return &errtypes.ClusterError{Msg: "refusing binary removal from critical path"}
	}
	var hasErrors bool

	pm := detectPackageManager(logger)

	if err := pm.Remove(ctx, InstalledPackages()); err != nil {
		logger.Warn("cleanup: some packages could not be removed (may require manual cleanup)", "err", err)
		hasErrors = true
	}

	for _, binary := range InstalledBinaries() {
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
			logger.Warn("cleanup: could not remove binary", "path", binPath, "err", err)
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
