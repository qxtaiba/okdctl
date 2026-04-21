package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okdctl/internal/system"
)

// PackageManager abstracts the host package manager (dnf or apt-get) used to
// install OKD host dependencies.
type PackageManager interface {
	Install(ctx context.Context, packages []string, logger *slog.Logger) error
	Remove(ctx context.Context, packages []string, logger *slog.Logger) error
	IsInstalled(ctx context.Context, pkg string) (bool, error)
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

// NewPackageManager returns a Manager wired to the appropriate backend for
// the detected OS family (dnf/rpm on RHEL, apt-get/dpkg on Debian).
func NewPackageManager(detected OS) PackageManager {
	if detected.Family == FamilyDebian {
		return &Manager{
			family:     FamilyDebian,
			pkgCmd:     "apt-get",
			queryCmd:   "dpkg",
			queryArgs:  []string{"-l"},
			queryMatch: "ii  ",
		}
	}
	return &Manager{
		family:    FamilyRHEL,
		pkgCmd:    "dnf",
		queryCmd:  "rpm",
		queryArgs: []string{"-q"},
	}
}

// Install installs packages via the configured backend. Empty input
// is a no-op.
func (m *Manager) Install(ctx context.Context, packages []string, logger *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	logger.Info(fmt.Sprintf("packages: installing %s", strings.Join(packages, ", ")))
	args := append([]string{"install", "-y"}, packages...)
	return runCaptured(ctx, m.pkgCmd, args)
}

// Remove uninstalls only the packages in packages that are currently
// installed, leaving the rest alone.
func (m *Manager) Remove(ctx context.Context, packages []string, _ *slog.Logger) error {
	if len(packages) == 0 {
		return nil
	}
	var installed []string
	for _, pkg := range packages {
		ok, err := m.IsInstalled(ctx, pkg)
		if err != nil {
			return fmt.Errorf("query %s: %w", pkg, err)
		}
		if ok {
			installed = append(installed, pkg)
		}
	}
	if len(installed) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, installed...)
	return runCaptured(ctx, m.pkgCmd, args)
}

// IsInstalled reports whether pkg is present via the backend's query
// command (for dpkg, stale "rc" entries are filtered). A non-zero exit
// maps to (false, nil); other failures (ctx cancellation, LookPath,
// I/O) propagate so callers don't treat a broken query backend as
// "not installed".
func (m *Manager) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	args := append(append([]string{}, m.queryArgs...), pkg)
	cmd := exec.CommandContext(ctx, m.queryCmd, args...) //nolint:gosec // queryCmd/queryArgs are set only from the literal constructors in NewPackageManager
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, fmt.Errorf("%s query: %w", m.queryCmd, err)
	}
	if m.queryMatch == "" {
		return true, nil
	}
	return strings.Contains(string(output), m.queryMatch+pkg), nil
}

// AddRepo registers a new package repository with the backend: dnf
// config-manager on RHEL, an /etc/apt/sources.list.d entry on Debian.
func (m *Manager) AddRepo(ctx context.Context, name, url string, logger *slog.Logger) error {
	logger.Info(fmt.Sprintf("packages: adding repository %s", name))

	if m.family == FamilyRHEL {
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
