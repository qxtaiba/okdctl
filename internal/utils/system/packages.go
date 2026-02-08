package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

func InstallPackages(ctx context.Context, packages []string, description string, logger utils.Logger) error {
	if len(packages) == 0 {
		return nil
	}

	logger.Info(fmt.Sprintf("packages: installing %s", description))
	logger.Info(fmt.Sprintf("packages: %s", strings.Join(packages, ", ")))

	args := append([]string{"install", "-y"}, packages...)
	if err := RunSudo(ctx, "dnf", args...); err != nil {
		logger.Error(fmt.Sprintf("packages: failed to install %s", description))
		return utils.WrapError("dnf install failed", err)
	}

	logger.Info(fmt.Sprintf("packages: %s installed", description))
	return nil
}

func RemovePackages(ctx context.Context, packages []string, logger utils.Logger) error {
	if len(packages) == 0 {
		return nil
	}

	installed := filterInstalledPackages(packages)
	if len(installed) == 0 {
		logger.Info("packages: none to remove (none installed)")
		return nil
	}

	logger.Info(fmt.Sprintf("packages: removing %s", strings.Join(installed, ", ")))

	args := append([]string{"remove", "-y"}, installed...)
	if err := RunSudo(ctx, "dnf", args...); err != nil {
		logger.Error(fmt.Sprintf("packages: failed to remove: %v", err))
		return utils.WrapError("dnf remove failed", err)
	}

	logger.Info("packages: removed successfully")
	return nil
}

func filterInstalledPackages(packages []string) []string {
	var installed []string
	for _, pkg := range packages {
		if isPackageInstalled(pkg) {
			installed = append(installed, pkg)
		}
	}
	return installed
}

func isPackageInstalled(pkg string) bool {
	cmd := exec.Command("rpm", "-q", pkg)
	return cmd.Run() == nil
}
