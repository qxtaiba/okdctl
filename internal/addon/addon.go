// Package addon provides a plugin system for optional cluster features.
// Addons self-register via init() and are resolved in dependency order.
package addon

import (
	"context"
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
)

type Addon interface {
	Info() AddonInfo

	Install(ctx context.Context, env *Environment) error
	Verify(ctx context.Context, env *Environment) error
	Uninstall(ctx context.Context, env *Environment) error
}

// AddonInfo is the metadata block an Addon returns from Info(). Name is the
// config key ("flux"); Priority is the install-order weight (lower first);
// Dependencies names other addons that must be installed first.
type AddonInfo struct {
	Name           string
	DisplayName    string
	Description    string
	Category       string
	Dependencies   []string
	Priority       int
	DefaultEnabled bool
}

type Environment struct {
	AddonConfig config.AddonConfig
	Exec        *executor.Executor
	Logger      *slog.Logger
	ProjectRoot string
}

type ConfigurableAddon interface {
	Addon

	DefaultSettings() map[string]string
	ValidateSettings(settings map[string]string) []string
}

type ToolSpec struct {
	Name        string
	Description string
}

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
