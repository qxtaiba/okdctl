package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/packages"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/platform"
)

// detectOS returns the detected host OS, falling back to RHEL if detection
// fails (cleanup runs on the bastion, which is typically RHEL-family).
func detectOS(logger *slog.Logger) platform.OS {
	detectedOS, err := platform.Detect()
	if err != nil {
		logutil.OrNop(logger).Warn("platform: detect failed; defaulting to rhel", "err", err)
		return platform.OS{Family: platform.FamilyRHEL, ID: "unknown", Version: ""}
	}
	return detectedOS
}

// detectPackageManager returns a PackageManager for the current host OS,
// falling back to RHEL/dnf if detection fails (cleanup runs on the bastion).
func detectPackageManager(logger *slog.Logger) platform.PackageManager {
	return platform.NewPackageManager(detectOS(logger))
}

// InstalledPackages returns the list of dnf packages installed by the setup phase.
// haproxy, httpd, dnsmasq are removed by their respective cleanup functions
// to ensure proper service stop/disable before removal.
func InstalledPackages() []string {
	return []string{
		"coreos-installer",
		"terraform",
	}
}

// InstalledBinaries returns the installer-managed binaries that Packages
// removes (OKD release binaries plus registered external tools).
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
// error only if at least one removal failed.
func Packages(ctx context.Context, logger *slog.Logger) error {
	logger = logutil.OrNop(logger)
	var hasErrors bool

	pm := detectPackageManager(logger)

	pkgList := InstalledPackages()
	if err := packages.Remove(ctx, pm, pkgList, logger); err != nil {
		logger.Warn("cleanup: some packages could not be removed (may require manual cleanup)")
		hasErrors = true
	}

	binaries := InstalledBinaries()
	for _, binary := range binaries {
		binPath := filepath.Join(phase.DefaultBinDir, binary)
		if _, err := os.Stat(binPath); os.IsNotExist(err) {
			continue // Already removed or never installed
		}
		if guardErr := refuseCriticalPath(binPath); guardErr != nil {
			logger.Warn(guardErr.Error())
			hasErrors = true
			continue
		}

		if err := os.RemoveAll(binPath); err != nil {
			logger.Warn("cleanup: failed to remove binary", "path", binPath, "err", err)
			hasErrors = true
		} else {
			logger.Info(fmt.Sprintf("cleanup: removed %s", binPath))
		}
	}

	if hasErrors {
		return &errtypes.ClusterError{Msg: "some cleanup operations failed"}
	}
	return nil
}
