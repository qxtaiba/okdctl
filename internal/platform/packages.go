// Package platform — see platform.go for the package summary. This file
// implements the OS-agnostic PackageManager abstraction, with a single
// Manager type driven by per-family binary names (dnf/apt-get, rpm/dpkg)
// rather than separate DNF/APT structs. Stderr is captured on every
// invocation so an install/remove failure surfaces the apt/dnf error
// message rather than a bare exit code.
package platform

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okdctl/internal/system"
)

type PackageManager interface {
	Install(ctx context.Context, packages []string, logger *slog.Logger) error
	Remove(ctx context.Context, packages []string, logger *slog.Logger) error
	IsInstalled(ctx context.Context, pkg string) bool
	AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error
}

// Manager is the single PackageManager implementation. The family field
// selects between RHEL (dnf/rpm) and Debian (apt-get/dpkg) binaries and
// drives the AddRepo branch.
type Manager struct {
	family    string
	pkgCmd    string   // "dnf" | "apt-get"
	queryCmd  string   // "rpm" | "dpkg"
	queryArgs []string // ["-q"] | ["-l"]
	// queryMatch is the substring that must appear in queryCmd stdout for a
	// package to count as installed. Empty → exit code alone is sufficient
	// (rpm -q exits 0 iff installed); non-empty → the exit must be 0 *and*
	// the output must contain this substring (dpkg -l prints stale entries
	// with "rc " state for purged packages).
	queryMatch string
}

func NewPackageManager(detected OS) PackageManager {
	if detected.Family == familyDebian {
		return &Manager{
			family:     familyDebian,
			pkgCmd:     "apt-get",
			queryCmd:   "dpkg",
			queryArgs:  []string{"-l"},
			queryMatch: "ii  ",
		}
	}
	return &Manager{
		family:    familyRHEL,
		pkgCmd:    "dnf",
		queryCmd:  "rpm",
		queryArgs: []string{"-q"},
	}
}

func (m *Manager) Install(ctx context.Context, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	logger.Info(fmt.Sprintf("packages: installing %s", strings.Join(packages, ", ")))
	args := append([]string{"install", "-y"}, packages...)
	return runCaptured(ctx, m.pkgCmd, args)
}

func (m *Manager) Remove(ctx context.Context, packages []string, _ *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	var installed []string
	for _, pkg := range packages {
		if m.IsInstalled(ctx, pkg) {
			installed = append(installed, pkg)
		}
	}
	if len(installed) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, installed...)
	return runCaptured(ctx, m.pkgCmd, args)
}

func (m *Manager) IsInstalled(ctx context.Context, pkg string) bool {
	args := append(append([]string{}, m.queryArgs...), pkg)
	cmd := exec.CommandContext(ctx, m.queryCmd, args...) //nolint:gosec // queryCmd/queryArgs are set only from the literal constructors in NewPackageManager
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	if m.queryMatch == "" {
		return true
	}
	return strings.Contains(string(output), m.queryMatch+pkg)
}

func (m *Manager) AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error {
	logger.Info(fmt.Sprintf("packages: adding repository %s", name))

	if m.family == familyRHEL {
		return runCaptured(ctx, m.pkgCmd, []string{"config-manager", "--add-repo", url})
	}

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

	if err := system.CopyFile(tmpPath, listPath); err != nil {
		return fmt.Errorf("failed to install repo list: %w", err)
	}
	return runCaptured(ctx, m.pkgCmd, []string{"update"})
}

// runCaptured runs a subprocess capturing stderr into the returned error so
// callers (setup, install flows) see the upstream apt/dnf message instead of
// a bare exit-code wrap. Stdout is discarded — package managers' stdout is
// progress text, not useful in an error path.
func runCaptured(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("%s %s: %w", bin, args[0], err)
		}
		return fmt.Errorf("%s %s: %w: %s", bin, args[0], err, msg)
	}
	return nil
}

func dpkgArch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "dpkg", "--print-architecture")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
