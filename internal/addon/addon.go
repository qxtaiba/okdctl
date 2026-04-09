// Package addon provides a plugin system for optional cluster features.
// Addons self-register via init() and are resolved in dependency order.
package addon

import (
	"context"
	"log/slog"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
)

type Addon interface {
	Info() AddonInfo

	Install(ctx context.Context, env *Environment) error
	Verify(ctx context.Context, env *Environment) error
	Uninstall(ctx context.Context, env *Environment) error
}

type AddonInfo struct {
	// Name is the unique identifier used in config (e.g., "flux").
	Name string

	// DisplayName is the human-readable name (e.g., "Flux GitOps").
	DisplayName string

	Description string

	// Category groups related addons (e.g., "networking", "gitops", "secrets").
	Category string

	// Dependencies lists addon names that must be installed first.
	Dependencies []string

	// Priority controls installation order (lower = installed first).
	Priority int

	DefaultEnabled bool
}

type Environment struct {
	AddonConfig config.AddonConfig

	Exec *executor.Executor

	Logger *slog.Logger

	ProjectRoot string
}

type ConfigurableAddon interface {
	Addon

	DefaultSettings() map[string]string
	ValidateSettings(settings map[string]string) []string
}

type ToolSpec struct {
	// Name is the binary name (e.g., "helm", "sops").
	Name string

	Description string
}

type ToolProvider interface {
	RequiredTools() []ToolSpec
}

// WizardField describes a single input field that an addon contributes to the
// interactive configuration wizard. Each field is rendered as a labelled input
// and its value is persisted into the addon's settings map under Key.
type WizardField struct {
	// Key is the settings map key the wizard writes to (e.g., "provider",
	// "repository"). It must match a key the addon recognises in its
	// ValidateSettings / Install logic.
	Key string

	// Label is the human-readable label displayed next to the input
	// (e.g., "GitOps Provider").
	Label string

	// Default is the initial value shown when the wizard opens. It is also
	// used as the persisted value if the user leaves the input untouched.
	Default string

	// Help is the hint text shown below the input to explain the field's
	// purpose or expected format.
	Help string

	// Required marks the field as mandatory when the addon is enabled;
	// the wizard refuses to advance while a required field is empty.
	Required bool
}

// WizardProvider is implemented by addons that want to expose configuration
// fields in the interactive wizard. The returned fields are rendered in the
// order given, and their values are written into the addon's settings map
// (the same map passed to ValidateSettings and surfaced via AddonConfig).
// Addons that do not implement WizardProvider are still selectable in the
// wizard but contribute no custom inputs.
type WizardProvider interface {
	WizardFields() []WizardField
}
