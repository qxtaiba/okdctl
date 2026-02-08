// Package addon provides a plugin system for optional cluster features.
// Addons self-register via init() and are resolved in dependency order.
package addon

import (
	"context"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
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
	Config *config.Config

	AddonConfig config.AddonConfig

	Exec *executor.Executor

	Logger utils.Logger

	// Outputs stores cross-addon data for dependent addons.
	Outputs *OutputStore

	ProjectRoot string
}

type ConfigurableAddon interface {
	Addon

	DefaultSettings() map[string]string
	ValidateSettings(settings map[string]string) []string
}

type OutputProducer interface {
	// Outputs returns key-value pairs available to dependent addons.
	Outputs() map[string]string
}

type ToolSpec struct {
	// Name is the binary name (e.g., "helm", "sops").
	Name string

	Description string
}

type ToolProvider interface {
	RequiredTools() []ToolSpec
}

type WizardField struct {
	// Key is the settings map key (e.g., "provider", "repository").
	Key string

	// Label is displayed to the user (e.g., "GitOps Provider").
	Label string

	Default string

	// Help is the hint text shown below the input.
	Help string

	// Required marks the field as mandatory when the addon is enabled.
	Required bool
}

type WizardProvider interface {
	WizardFields() []WizardField
}
