package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/packages"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/phase"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/setup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/platform"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// detectOS returns the detected host OS, falling back to RHEL if detection
// fails (cleanup runs on the bastion, which is typically RHEL-family).
func detectOS(logger *slog.Logger) platform.OS {
	detectedOS, err := platform.Detect()
	if err != nil {
		if logger != nil {
			logger.Warn(fmt.Sprintf("platform: %v, defaulting to rhel", err))
		}
		return platform.OS{Family: "rhel", ID: "unknown", Version: ""}
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

func InstalledBinaries() []string {
	okdBinaries := []string{
		"openshift-install",
		"oc",
		"kubectl",
	}
	return append(okdBinaries, setup.ExternalToolBinaries()...)
}

func Packages(ctx context.Context, logger *slog.Logger) error {
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

		if err := system.RemoveAll(ctx, binPath, fmt.Sprintf("remove %s", binary)); err != nil {
			logger.Warn(fmt.Sprintf("cleanup: failed to remove %s: %v", binPath, err))
			hasErrors = true
		} else {
			logger.Info(fmt.Sprintf("cleanup: removed %s", binPath))
		}
	}

	if hasErrors {
		return fmt.Errorf("some cleanup operations failed")
	}
	return nil
}
