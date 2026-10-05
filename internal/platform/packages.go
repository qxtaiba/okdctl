package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// packageManagerTimeout bounds a package-manager invocation — dnf against a
// wedged mirror can hang indefinitely otherwise; 15m mirrors
// ocExtractTimeout's posture (setup/release_extract.go).
const packageManagerTimeout = 15 * time.Minute

// Manager installs and removes OKD host dependencies through dnf, querying
// rpm for what is present. Must be constructed via NewPackageManager — the
// zero value panics on first use.
type Manager struct {
	logger *slog.Logger
}

// NewPackageManager returns a dnf/rpm Manager; a nil logger falls back to
// logutil.NopLogger.
func NewPackageManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logutil.OrNop(logger)}
}

// Install installs packages via dnf; empty input is a no-op.
func (m *Manager) Install(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	m.logger.Info("packages: installing", "packages", packages)
	installCtx, cancel := context.WithTimeout(ctx, packageManagerTimeout)
	defer cancel()
	args := append([]string{"install", "-y"}, packages...)
	return executor.RunCaptured(installCtx, "dnf", args...)
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
	return executor.RunCaptured(removeCtx, "dnf", args...)
}

// isInstalled reports whether pkg is present via `rpm -q`; a non-zero exit
// maps to (false, nil), other failures propagate so a broken rpm isn't
// mistaken for "not installed".
func (m *Manager) isInstalled(ctx context.Context, pkg string) (bool, error) {
	if _, err := executor.OutputCaptured(ctx, "rpm", "-q", pkg); err != nil {
		var exitErr *executor.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, fmt.Errorf("rpm query: %w", err)
	}
	return true, nil
}
