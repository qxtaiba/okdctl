package addon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
)

type Manager struct {
	cfg         *config.Config
	exec        *executor.Executor
	logger      *slog.Logger
	projectRoot string
}

func NewManager(cfg *config.Config, exec *executor.Executor, logger *slog.Logger, projectRoot string) *Manager {
	return &Manager{
		cfg:         cfg,
		exec:        exec,
		logger:      logger,
		projectRoot: projectRoot,
	}
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
		return fmt.Errorf("addons: 'oc' binary is required but not found in PATH")
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

		if dep := m.firstFailedDep(info.Dependencies, failed); dep != "" {
			m.logger.Warn(fmt.Sprintf("addons: skipping %s (dependency %s failed)", info.DisplayName, dep))
			failed[info.Name] = true
			continue
		}

		env, err := m.installAndVerify(ctx, a)
		if err != nil {
			failed[info.Name] = true
			m.logger.Warn(err.Error())
			errs = append(errs, err)

			m.logger.Info(fmt.Sprintf("addons: rolling back %s", info.DisplayName))
			if unErr := a.Uninstall(ctx, env); unErr != nil {
				m.logger.Warn(fmt.Sprintf("addons: rollback of %s failed: %v", info.DisplayName, unErr))
				errs = append(errs, fmt.Errorf("addon %s rollback: %w", info.Name, unErr))
			}
			continue
		}
	}

	return errors.Join(errs...)
}

// installAndVerify runs Install + Verify for a single addon, returning the
// Environment on success or error. Used by both InstallAll and InstallOne.
func (m *Manager) installAndVerify(ctx context.Context, a Addon) (*Environment, error) {
	info := a.Info()
	m.logger.Info(fmt.Sprintf("addons: installing %s", info.DisplayName))
	env := m.buildEnv(a)
	if err := a.Install(ctx, env); err != nil {
		return env, fmt.Errorf("addon %s install failed: %w", info.Name, err)
	}
	if vErr := a.Verify(ctx, env); vErr != nil {
		m.logger.Warn(fmt.Sprintf("addons: %s installed but verify failed: %v", info.DisplayName, vErr))
	} else {
		m.logger.Info(fmt.Sprintf("addons: %s installed and verified", info.DisplayName))
	}
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
		return fmt.Errorf("unknown addon: %s", name)
	}

	if !executor.CommandExists("oc") {
		return fmt.Errorf("addons: 'oc' binary is required but not found in PATH")
	}

	toInstall, err := m.collectWithDeps(a)
	if err != nil {
		return err
	}

	ordered, err := Resolve(toInstall)
	if err != nil {
		return err
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
			for i := len(installed) - 1; i >= 0; i-- {
				inst := installed[i]
				m.logger.Info(fmt.Sprintf("addons: rolling back %s", inst.a.Info().DisplayName))
				if unErr := inst.a.Uninstall(ctx, inst.env); unErr != nil {
					m.logger.Warn(fmt.Sprintf("addons: rollback of %s failed: %v", inst.a.Info().DisplayName, unErr))
					err = errors.Join(err, fmt.Errorf("addon %s rollback: %w", inst.a.Info().Name, unErr))
				}
			}
			return err
		}
		installed = append(installed, installedAddon{a: addon, env: env})
	}

	return nil
}

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
		AddonConfig: ac,
		Exec:        m.exec,
		Logger:      m.logger,
		ProjectRoot: m.projectRoot,
	}
}
