package platform

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

// PackageManager abstracts system package management across OS families.
type PackageManager interface {
	Install(ctx context.Context, packages []string, logger *slog.Logger) error
	Remove(ctx context.Context, packages []string, logger *slog.Logger) error
	IsInstalled(pkg string) bool
	AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error
}

// NewPackageManager returns the appropriate PackageManager for the detected OS.
func NewPackageManager(os OS) PackageManager {
	if os.Family == familyDebian {
		return &APTManager{}
	}
	return &DNFManager{}
}

// DNFManager wraps dnf/rpm for RHEL-family systems.
type DNFManager struct{}

func (m *DNFManager) Install(ctx context.Context, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	logger.Info(fmt.Sprintf("packages: installing %s", strings.Join(packages, ", ")))
	args := append([]string{"install", "-y"}, packages...)
	if err := system.RunSudo(ctx, "dnf", args...); err != nil {
		return fmt.Errorf("dnf install failed: %w", err)
	}
	return nil
}

func (m *DNFManager) Remove(ctx context.Context, packages []string, _ *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	var installed []string
	for _, pkg := range packages {
		if m.IsInstalled(pkg) {
			installed = append(installed, pkg)
		}
	}
	if len(installed) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, installed...)
	return system.RunSudo(ctx, "dnf", args...)
}

func (m *DNFManager) IsInstalled(pkg string) bool {
	return exec.CommandContext(context.Background(), "rpm", "-q", pkg).Run() == nil
}

func (m *DNFManager) AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error {
	logger.Info(fmt.Sprintf("packages: adding repository %s", name))
	return system.RunSudo(ctx, "dnf", "config-manager", "--add-repo", url)
}

// APTManager wraps apt-get/dpkg for Debian-family systems.
type APTManager struct{}

func (m *APTManager) Install(ctx context.Context, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	logger.Info(fmt.Sprintf("packages: installing %s", strings.Join(packages, ", ")))
	args := append([]string{"install", "-y"}, packages...)
	if err := system.RunSudo(ctx, "apt-get", args...); err != nil {
		return fmt.Errorf("apt-get install failed: %w", err)
	}
	return nil
}

func (m *APTManager) Remove(ctx context.Context, packages []string, _ *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	var installed []string
	for _, pkg := range packages {
		if m.IsInstalled(pkg) {
			installed = append(installed, pkg)
		}
	}
	if len(installed) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, installed...)
	return system.RunSudo(ctx, "apt-get", args...)
}

func (m *APTManager) IsInstalled(pkg string) bool {
	cmd := exec.CommandContext(context.Background(), "dpkg", "-l", pkg)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "ii  "+pkg)
}

func (m *APTManager) AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error {
	logger.Info(fmt.Sprintf("packages: adding repository %s", name))
	return system.RunSudo(ctx, "sh", "-c",
		fmt.Sprintf("echo 'deb [arch=$(dpkg --print-architecture)] %s any main' > /etc/apt/sources.list.d/%s.list && apt-get update", url, name))
}
