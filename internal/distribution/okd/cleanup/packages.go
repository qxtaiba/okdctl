package cleanup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/setup"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// InstalledPackages returns the list of dnf packages installed by the setup phase.
// Only includes packages not already handled by service-specific cleanup functions.
// Note: haproxy, httpd, dnsmasq are removed by their respective cleanup functions
// (HAProxy(), Apache(), Dnsmasq()) to ensure proper service stop/disable before removal.
func InstalledPackages() []string {
	return []string{
		// OKD-specific tools
		"coreos-installer",
		// Terraform (installed via HashiCorp repo)
		"terraform",
	}
}

// InstalledBinaries returns the list of binaries installed to /usr/local/bin by setup.
// This includes OKD tools and external tools (terraform, yq, age, sops).
func InstalledBinaries() []string {
	okdBinaries := []string{
		"openshift-install",
		"oc",
		"kubectl",
	}
	return append(okdBinaries, setup.ExternalToolBinaries()...)
}

// Packages removes all system packages and binaries installed during setup.
// This restores the system to its pre-installation state.
func Packages(ctx context.Context, logger logging.Logger) error {
	var hasErrors bool

	packages := InstalledPackages()
	if err := system.RemovePackages(ctx, packages, logger); err != nil {
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
