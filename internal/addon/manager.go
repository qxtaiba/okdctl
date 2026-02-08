package addon

import (
	"context"
	"errors"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

// Manager orchestrates addon lifecycle operations.
type Manager struct {
	cfg         *config.Config
	exec        *executor.Executor
	logger      logging.Logger
	outputs     *OutputStore
	projectRoot string
}

// NewManager creates an addon manager.
func NewManager(cfg *config.Config, exec *executor.Executor, logger logging.Logger, projectRoot string) *Manager {
	return &Manager{
		cfg:         cfg,
		exec:        exec,
		logger:      logger,
		outputs:     NewOutputStore(),
		projectRoot: projectRoot,
	}
}

// OutputStore returns the manager's output store for reading addon outputs.
func (m *Manager) OutputStore() *OutputStore {
	return m.outputs
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

	ordered, err := Resolve(enabled)
	if err != nil {
		return fmt.Errorf("addon dependency resolution failed: %w", err)
	}

	m.logger.Info(fmt.Sprintf("addons: installing %d addon(s)", len(ordered)))

	failed := make(map[string]bool)
	var errs []error

	for _, a := range ordered {
		info := a.Info()
		if err := ctx.Err(); err != nil {
			return err
		}

		// Skip if any dependency failed.
		if dep := m.firstFailedDep(info.Dependencies, failed); dep != "" {
			m.logger.Warn(fmt.Sprintf("addons: skipping %s (dependency %s failed)", info.DisplayName, dep))
			failed[info.Name] = true
			continue
		}

		m.logger.Info(fmt.Sprintf("addons: installing %s", info.DisplayName))

		env := m.buildEnv(a)
		if err := a.Install(ctx, env); err != nil {
			failed[info.Name] = true
			addonErr := fmt.Errorf("addon %s install failed: %w", info.Name, err)
			m.logger.Warn(addonErr.Error())
			errs = append(errs, addonErr)
			continue
		}

		if op, ok := a.(OutputProducer); ok {
			for k, v := range op.Outputs() {
				m.outputs.Set(info.Name, k, v)
			}
		}

		m.logger.Info(fmt.Sprintf("addons: %s installed", info.DisplayName))
	}

	return errors.Join(errs...)
}

// firstFailedDep returns the name of the first dependency in the failed set, or "".
func (m *Manager) firstFailedDep(deps []string, failed map[string]bool) string {
	for _, d := range deps {
		if failed[d] {
			return d
		}
	}
	return ""
}

// InstallOne installs a single addon plus any missing dependencies.
func (m *Manager) InstallOne(ctx context.Context, name string) error {
	a := Get(name)
	if a == nil {
		return fmt.Errorf("unknown addon: %s", name)
	}

	toInstall, err := m.collectWithDeps(a)
	if err != nil {
		return err
	}

	ordered, err := Resolve(toInstall)
	if err != nil {
		return err
	}

	for _, addon := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}

		info := addon.Info()
		m.logger.Info(fmt.Sprintf("addons: installing %s", info.DisplayName))

		env := m.buildEnv(addon)
		if err := addon.Install(ctx, env); err != nil {
			return fmt.Errorf("addon %s install failed: %w", info.Name, err)
		}

		if op, ok := addon.(OutputProducer); ok {
			for k, v := range op.Outputs() {
				m.outputs.Set(info.Name, k, v)
			}
		}
	}

	return nil
}

// VerifyAll checks all enabled addons.
func (m *Manager) VerifyAll(ctx context.Context) error {
	enabled := Enabled(m.cfg)
	for _, a := range enabled {
		if err := ctx.Err(); err != nil {
			return err
		}

		info := a.Info()
		env := m.buildEnv(a)
		if err := a.Verify(ctx, env); err != nil {
			return fmt.Errorf("addon %s verify failed: %w", info.Name, err)
		}
	}
	return nil
}

// Uninstall removes an addon, blocking if dependents are still enabled.
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	a := Get(name)
	if a == nil {
		return fmt.Errorf("unknown addon: %s", name)
	}

	for _, other := range Enabled(m.cfg) {
		for _, dep := range other.Info().Dependencies {
			if dep == name {
				return fmt.Errorf("cannot uninstall %s: %s depends on it", name, other.Info().Name)
			}
		}
	}

	env := m.buildEnv(a)
	return a.Uninstall(ctx, env)
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
				return fmt.Errorf("addon %s requires %s which is not registered", info.Name, depName)
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
	ac := m.cfg.Addons[a.Info().Name]
	return &Environment{
		Config:      m.cfg,
		AddonConfig: ac,
		Exec:        m.exec,
		Logger:      m.logger,
		Outputs:     m.outputs,
		ProjectRoot: m.projectRoot,
	}
}
