package addon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Manager drives the addon lifecycle: resolving dependencies, installing in
// order, verifying, and rolling back on failure. Construct via NewManager
// with functional options.
type Manager struct {
	cfg         *config.Config
	exec        *executor.Executor
	logger      *slog.Logger
	projectRoot string
}

// Option configures a Manager at construction time.
type Option func(*Manager)

// WithExecutor sets the subprocess executor used by addon Install/Verify/Uninstall.
func WithExecutor(exec *executor.Executor) Option {
	return func(m *Manager) { m.exec = exec }
}

// WithLogger attaches a logger. Nil resolves to NopLogger.
func WithLogger(l *slog.Logger) Option {
	return func(m *Manager) { m.logger = logutil.OrNop(l) }
}

// WithProjectRoot sets the path the manager resolves addon-local resources
// against (manifests, helm charts, flux bootstrap paths).
func WithProjectRoot(root string) Option {
	return func(m *Manager) { m.projectRoot = root }
}

// NewManager constructs a Manager bound to cfg with options applied in order.
// A nil logger is tolerated and resolved to NopLogger so the body can log
// unconditionally — matches the nil-safety contract of phase.NewBasePhase.
func NewManager(cfg *config.Config, opts ...Option) *Manager {
	m := &Manager{cfg: cfg}
	for _, opt := range opts {
		opt(m)
	}
	if m.logger == nil {
		m.logger = logutil.NopLogger
	}
	if m.exec == nil {
		m.exec = executor.New(executor.WithLogger(m.logger))
	}
	return m
}

// InstallAll resolves and installs all enabled addons in dependency order.
// Independent addons are attempted even if an earlier addon fails; addons
// whose dependency failed are skipped.
func (m *Manager) InstallAll(ctx context.Context) error {
	enabled := Enabled(m.cfg)
	if len(enabled) == 0 {
		m.logger.Info("addons: no addons enabled, skipping")
		return nil
	}

	if !executor.CommandExists("oc") {
		return &errtypes.ClusterError{Msg: "addons: 'oc' binary is required but not found in PATH"}
	}

	ordered, err := Resolve(enabled)
	if err != nil {
		return &errtypes.ConfigError{Msg: "addon dependency resolution failed", Err: err}
	}

	m.logger.Info("addons: installing", "count", len(ordered))

	failed := make(map[string]bool)
	var errs []error

	for _, a := range ordered {
		info := a.Info()
		// Bare ctx.Err so cli/root.go::signalExitCode resolves SIGINT→130
		// without a typed wrap; once any addon fails errs is non-empty and
		// this branch is unreachable — L110 joins ctxErr into the aggregate.
		if err := ctx.Err(); err != nil {
			return err
		}

		if dep := m.firstFailedDep(info.Dependencies, failed); dep != "" {
			m.logger.Warn("addons: skipping — dependency failed", "addon", info.DisplayName, "dep", dep)
			failed[info.Name] = true
			continue
		}

		env, err := m.installAndVerify(ctx, a)
		if err != nil {
			failed[info.Name] = true
			m.logger.Warn("addons: install and verify failed", "addon", info.DisplayName, "err", err)
			errs = append(errs, err)

			m.logger.Info("addons: rolling back", "addon", info.DisplayName)
			if unErr := a.Uninstall(ctx, env); unErr != nil {
				m.logger.Warn("addons: rollback failed", "addon", info.DisplayName, "err", unErr)
				errs = append(errs, fmt.Errorf("addon %s rollback: %w", info.Name, unErr))
			}
			continue
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil && len(errs) > 0 {
		errs = append(errs, ctxErr)
	}
	return errors.Join(errs...)
}

// installAndVerify runs Install + Verify for a single addon.
// Verify failure fails the install — the addon is rolled back by the caller.
func (m *Manager) installAndVerify(ctx context.Context, a Addon) (*Environment, error) {
	info := a.Info()
	m.logger.Info("addons: installing addon", "addon", info.DisplayName)
	env := m.buildEnv(a)
	if err := a.Install(ctx, env); err != nil {
		return env, &errtypes.ClusterError{Msg: fmt.Sprintf("addon %s install failed", info.Name), Err: err}
	}
	if vErr := a.Verify(ctx, env); vErr != nil {
		return env, &errtypes.ClusterError{Msg: fmt.Sprintf("addon %s installed but verify failed", info.Name), Err: vErr}
	}
	m.logger.Info("addons: installed and verified", "addon", info.DisplayName)
	return env, nil
}

func (m *Manager) firstFailedDep(deps []string, failed map[string]bool) string {
	for _, d := range deps {
		if failed[d] {
			return d
		}
	}
	return ""
}

// InstallOne installs a single addon plus any missing dependencies.
//
// Rollback semantics differ from InstallAll: this method is all-or-nothing.
// If any addon in the resolved dependency closure fails to install, every
// previously-installed addon in this call is uninstalled in reverse order
// and the method returns the aggregated error. InstallAll, by contrast, uses
// per-addon continuation: a failed addon is rolled back in isolation while
// unrelated addons continue installing.
func (m *Manager) InstallOne(ctx context.Context, name string) error {
	a := Get(name)
	if a == nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("unknown addon: %s", name)}
	}

	if !executor.CommandExists("oc") {
		return &errtypes.ClusterError{Msg: "addons: 'oc' binary is required but not found in PATH"}
	}

	toInstall, err := m.collectWithDeps(a)
	if err != nil {
		return err
	}

	ordered, err := Resolve(toInstall)
	if err != nil {
		return &errtypes.ConfigError{Msg: "addon dependency resolution failed", Err: err}
	}

	type installedAddon struct {
		a   Addon
		env *Environment
	}
	var installed []installedAddon

	for _, addon := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}

		env, err := m.installAndVerify(ctx, addon)
		if err != nil {
			// All-or-nothing: roll back previously-installed addons in reverse order.
			for _, inst := range slices.Backward(installed) {
				m.logger.Info("addons: rolling back", "addon", inst.a.Info().DisplayName)
				if unErr := inst.a.Uninstall(ctx, inst.env); unErr != nil {
					m.logger.Warn("addons: rollback failed", "addon", inst.a.Info().DisplayName, "err", unErr)
					err = errors.Join(err, fmt.Errorf("addon %s rollback: %w", inst.a.Info().Name, unErr))
				}
			}
			return err
		}
		installed = append(installed, installedAddon{a: addon, env: env})
	}

	return nil
}

// VerifyResult is one addon's verify outcome. Err is nil on success.
type VerifyResult struct {
	Name string
	Err  error
}

// VerifyAll runs Verify on every enabled addon and returns one result per
// addon plus an aggregated error built from any failures. Iteration does
// not stop on the first failure — callers receive results for every addon
// that was attempted before context cancellation.
func (m *Manager) VerifyAll(ctx context.Context) ([]VerifyResult, error) {
	enabled := Enabled(m.cfg)
	results := make([]VerifyResult, 0, len(enabled))
	var errs []error
	for _, a := range enabled {
		if err := ctx.Err(); err != nil {
			return results, err
		}

		info := a.Info()
		env := m.buildEnv(a)
		if vErr := a.Verify(ctx, env); vErr != nil {
			wrapped := &errtypes.ClusterError{Msg: fmt.Sprintf("addon %s verify failed", info.Name), Err: vErr}
			results = append(results, VerifyResult{Name: info.Name, Err: wrapped})
			errs = append(errs, wrapped)
		} else {
			results = append(results, VerifyResult{Name: info.Name})
		}
	}
	return results, errors.Join(errs...)
}

// Uninstall removes an addon, blocking if any enabled addon transitively
// depends on it.
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	a := Get(name)
	if a == nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("unknown addon: %s", name)}
	}

	if dep := m.findTransitiveDependent(name); dep != "" {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("cannot uninstall %s: %s depends on it (directly or transitively)", name, dep)}
	}

	env := m.buildEnv(a)
	return a.Uninstall(ctx, env)
}

// findTransitiveDependent returns the name of the first enabled addon that
// transitively depends on target, or "" if none do.
func (m *Manager) findTransitiveDependent(target string) string {
	for _, other := range Enabled(m.cfg) {
		if m.dependsOn(other.Info().Name, target, make(map[string]bool)) {
			return other.Info().Name
		}
	}
	return ""
}

func (m *Manager) dependsOn(addonName, target string, visited map[string]bool) bool {
	if visited[addonName] {
		return false
	}
	visited[addonName] = true

	a := Get(addonName)
	if a == nil {
		return false
	}
	deps := a.Info().Dependencies
	if slices.Contains(deps, target) {
		return true
	}
	for _, dep := range deps {
		if m.dependsOn(dep, target, visited) {
			return true
		}
	}
	return false
}

// collectWithDeps returns the addon and all its transitive dependencies.
func (m *Manager) collectWithDeps(a Addon) ([]Addon, error) {
	seen := make(map[string]bool)
	var result []Addon

	var visit func(Addon) error
	visit = func(addon Addon) error {
		info := addon.Info()
		if seen[info.Name] {
			return nil
		}
		seen[info.Name] = true

		for _, depName := range info.Dependencies {
			dep := Get(depName)
			if dep == nil {
				return &errtypes.ConfigError{Msg: fmt.Sprintf("addon %s requires %s which is not registered", info.Name, depName)}
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		result = append(result, addon)
		return nil
	}

	if err := visit(a); err != nil {
		return nil, err
	}
	return result, nil
}

func (m *Manager) buildEnv(a Addon) *Environment {
	return &Environment{
		AddonConfig: m.cfg.Addons[a.Info().Name],
		Exec:        m.exec,
		Logger:      m.logger,
		ProjectRoot: m.projectRoot,
	}
}
