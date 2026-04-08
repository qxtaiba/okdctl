// Package packages wraps dnf install/remove operations for OKD host
// dependencies (haproxy, dnsmasq, httpd, and related tooling).
package packages

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

func Install(ctx context.Context, packages []string, description string, logger utils.Logger) error {
	if len(packages) == 0 {
		return nil
	}

	logger.Info(fmt.Sprintf("packages: installing %s", description))
	logger.Info(fmt.Sprintf("packages: %s", strings.Join(packages, ", ")))

	args := append([]string{"install", "-y"}, packages...)
	if err := system.RunSudo(ctx, "dnf", args...); err != nil {
		return fmt.Errorf("dnf install failed: %w", err)
	}

	logger.Info(fmt.Sprintf("packages: %s installed", description))
	return nil
}

func Remove(ctx context.Context, packages []string, logger utils.Logger) error {
	if len(packages) == 0 {
		return nil
	}

	installed := filterInstalled(packages)
	if len(installed) == 0 {
		logger.Info("packages: none to remove (none installed)")
		return nil
	}

	logger.Info(fmt.Sprintf("packages: removing %s", strings.Join(installed, ", ")))

	args := append([]string{"remove", "-y"}, installed...)
	if err := system.RunSudo(ctx, "dnf", args...); err != nil {
		return fmt.Errorf("dnf remove failed: %w", err)
	}

	logger.Info("packages: removed successfully")
	return nil
}

func filterInstalled(packages []string) []string {
	var installed []string
	for _, pkg := range packages {
		if isInstalled(pkg) {
			installed = append(installed, pkg)
		}
	}
	return installed
}

func isInstalled(pkg string) bool {
	cmd := exec.Command("rpm", "-q", pkg)
	return cmd.Run() == nil
}
