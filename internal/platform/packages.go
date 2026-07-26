package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// packageManagerTimeout bounds a single package-manager invocation. dnf/apt
// against a wedged mirror or stale repo metadata can otherwise hang the
// underlying exec indefinitely; 15 minutes mirrors ocExtractTimeout's
// generous-but-bounded posture in setup/release_extract.go.
const packageManagerTimeout = 15 * time.Minute

// Manager is the host package manager (dnf or apt-get) used to install OKD
// host dependencies. The family field selects between RHEL (dnf/rpm) and
// Debian (apt-get/dpkg) binaries.
//
// Must be constructed via NewPackageManager — the zero value panics on
// first use (the backend commands and logger are set only there).
type Manager struct {
	family    Family
	pkgCmd    string                               // "dnf" | "apt-get"
	queryCmd  string                               // "rpm" | "dpkg"
	queryArgs []string                             // ["-q"] | ["-l"]
	postCheck func(stdout []byte, pkg string) bool // nil → exit code alone is sufficient
	logger    *slog.Logger
}

// NewPackageManager returns a Manager wired to the appropriate backend for
// the detected OS family (dnf/rpm on RHEL, apt-get/dpkg on Debian). A nil
// logger falls back to logutil.NopLogger.
func NewPackageManager(detected OS, logger *slog.Logger) *Manager {
	logger = logutil.OrNop(logger)
	if detected.Family == FamilyDebian {
		return &Manager{
			family:    FamilyDebian,
			pkgCmd:    "apt-get",
			queryCmd:  "dpkg",
			queryArgs: []string{"-l"},
			postCheck: func(stdout []byte, pkg string) bool {
				return bytes.Contains(stdout, []byte("ii  "+pkg))
			},
			logger: logger,
		}
	}
	return &Manager{
		family:    FamilyRHEL,
		pkgCmd:    "dnf",
		queryCmd:  "rpm",
		queryArgs: []string{"-q"},
		logger:    logger,
	}
}

// Install installs packages via the configured backend. Empty input
// is a no-op.
func (m *Manager) Install(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	m.logger.Info("packages: installing", "packages", packages)
	installCtx, cancel := context.WithTimeout(ctx, packageManagerTimeout)
	defer cancel()
	args := append([]string{"install", "-y"}, packages...)
	return executor.RunCaptured(installCtx, m.pkgCmd, args...)
}

// Remove uninstalls only the packages in packages that are currently
// installed, leaving the rest alone.
func (m *Manager) Remove(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	var installed []string
	for _, pkg := range packages {
		ok, err := m.isInstalled(ctx, pkg)
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
	removeCtx, cancel := context.WithTimeout(ctx, packageManagerTimeout)
	defer cancel()
	args := append([]string{"remove", "-y"}, installed...)
	return executor.RunCaptured(removeCtx, m.pkgCmd, args...)
}

// isInstalled reports whether pkg is present via the backend's query
// command (for dpkg, stale "rc" entries are filtered). A non-zero exit
// maps to (false, nil); other failures (ctx cancellation, LookPath,
// I/O) propagate so callers don't treat a broken query backend as
// "not installed".
func (m *Manager) isInstalled(ctx context.Context, pkg string) (bool, error) {
	args := slices.Concat(m.queryArgs, []string{pkg})
	output, err := executor.OutputCaptured(ctx, m.queryCmd, args...)
	if err != nil {
		var exitErr *executor.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, fmt.Errorf("%s query: %w", m.queryCmd, err)
	}
	if m.postCheck == nil {
		return true, nil
	}
	return m.postCheck(output, pkg), nil
}
