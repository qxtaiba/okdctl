package platform

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okdctl/internal/system"
)

// PackageManager abstracts system package management across OS families.
type PackageManager interface {
	Install(ctx context.Context, packages []string, logger *slog.Logger) error
	Remove(ctx context.Context, packages []string, logger *slog.Logger) error
	IsInstalled(pkg string) bool
	AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error
}

func NewPackageManager(detected OS) PackageManager {
	if detected.Family == familyDebian {
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

	arch, err := dpkgArch(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect architecture: %w", err)
	}

	listContent := fmt.Sprintf("deb [arch=%s] %s any main\n", arch, url)
	listPath := fmt.Sprintf("/etc/apt/sources.list.d/%s.list", name)

	tmpPath, err := system.WriteTempFile("apt-repo", 0o644, func(f *os.File) error {
		_, err := f.WriteString(listContent)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to write repo list: %w", err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if err := system.CopyFileWithElevation(ctx, tmpPath, listPath, "add apt repository"); err != nil {
		return fmt.Errorf("failed to install repo list: %w", err)
	}
	return system.RunSudo(ctx, "apt-get", "update")
}

func dpkgArch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "dpkg", "--print-architecture")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
