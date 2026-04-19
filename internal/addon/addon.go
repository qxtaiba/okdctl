// Package addon provides a plugin system for optional cluster features.
// Addons self-register via init() and are resolved in dependency order.
package addon

import (
	"context"
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// Addon is the contract every pluggable cluster feature must satisfy.
// Implementations self-register via init() and are installed in dependency
// order by Manager.
type Addon interface {
	Info() AddonInfo

	Install(ctx context.Context, env *Environment) error
	Verify(ctx context.Context, env *Environment) error
	Uninstall(ctx context.Context, env *Environment) error
}

// AddonInfo is the metadata block an Addon returns from Info(). Name is the
// config key ("flux"); Priority is the install-order weight (lower first);
// Dependencies names other addons that must be installed first.
//
//nolint:revive // stutter-named type is the established public API; rename is a breaking change tracked separately
type AddonInfo struct {
	Name           string
	DisplayName    string
	Description    string
	Category       string
	Dependencies   []string
	Priority       int
	DefaultEnabled bool
}

// Environment carries the shared dependencies an addon needs during Install,
// Verify, or Uninstall: its config slice, an executor for shelling out, a
// logger, and the project root for locating embedded assets.
type Environment struct {
	AddonConfig config.AddonConfig
	Exec        *executor.Executor
	Logger      *slog.Logger
	ProjectRoot string
}

// ConfigurableAddon is an Addon that exposes tunable settings with defaults
// and per-key validation.
type ConfigurableAddon interface {
	Addon

	DefaultSettings() map[string]string
	ValidateSettings(settings map[string]string) []string
}

// ToolSpec names an external binary an addon requires and why.
type ToolSpec struct {
	Name        string
	Description string
}

// ToolProvider is implemented by addons that require specific external tools
// on the host. The installer surfaces missing tools before install begins.
type ToolProvider interface {
	RequiredTools() []ToolSpec
}

// WizardField describes an input field an addon contributes to the wizard.
// Key is the settings map key; Required = true blocks wizard progress until
// populated.
type WizardField struct {
	Key      string
	Label    string
	Default  string
	Help     string
	Required bool
}

// WizardProvider is implemented by addons that expose configuration fields
// in the interactive wizard. Fields render in the returned order; their
// values land in AddonConfig.Settings keyed by WizardField.Key.
type WizardProvider interface {
	WizardFields() []WizardField
}
