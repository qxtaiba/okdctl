package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/packages"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/setup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

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

	pkgList := InstalledPackages()
	if err := packages.Remove(ctx, pkgList, logger); err != nil {
		logger.Warn("cleanup: some packages could not be removed (may require manual cleanup)")
		hasErrors = true
	}

	binaries := InstalledBinaries()
	for _, binary := range binaries {
		binPath := filepath.Join("/usr/local/bin", binary)
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
