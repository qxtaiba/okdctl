// Package addon provides a plugin system for optional cluster features.
// Addons self-register via init() and are resolved in dependency order.
package addon

import (
	"context"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/executor"
	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

// Addon is the core interface that all addons must implement.
type Addon interface {
	// Info returns metadata about the addon.
	Info() AddonInfo

	// Install deploys the addon into the cluster.
	Install(ctx context.Context, env *Environment) error

	// Verify checks whether the addon is healthy.
	Verify(ctx context.Context, env *Environment) error

	// Uninstall removes the addon from the cluster.
	Uninstall(ctx context.Context, env *Environment) error
}

// AddonInfo describes an addon's identity and behavior.
type AddonInfo struct {
	// Name is the unique identifier used in config (e.g., "metallb").
	Name string

	// DisplayName is the human-readable name (e.g., "MetalLB").
	DisplayName string

	// Description is a short summary of what the addon does.
	Description string

	// Category groups related addons (e.g., "networking", "gitops", "secrets").
	Category string

	// Dependencies lists addon names that must be installed first.
	Dependencies []string

	// Priority controls installation order (lower = installed first).
	Priority int

	// DefaultEnabled determines whether the addon is enabled in a fresh config.
	DefaultEnabled bool
}

// Environment provides shared resources to addons during lifecycle operations.
type Environment struct {
	// Config is the full cluster configuration.
	Config *config.Config

	// AddonConfig is this addon's specific configuration.
	AddonConfig config.AddonConfig

	// Exec runs shell commands against the cluster.
	Exec *executor.Executor

	// Logger provides structured logging.
	Logger logging.Logger

	// Outputs stores cross-addon data (e.g., MetalLB pool → Ingress).
	Outputs *OutputStore

	// ProjectRoot is the root directory of the project.
	ProjectRoot string
}

// ConfigurableAddon is implemented by addons that have settings to validate.
type ConfigurableAddon interface {
	Addon

	// DefaultSettings returns the default settings map for this addon.
	DefaultSettings() map[string]string

	// ValidateSettings checks that the addon's settings are valid.
	ValidateSettings(settings map[string]string) []string
}

// OutputProducer is implemented by addons that expose data for other addons.
type OutputProducer interface {
	// Outputs returns key-value pairs available to dependent addons.
	Outputs() map[string]string
}

// ToolSpec describes an external tool required by an addon.
type ToolSpec struct {
	// Name is the binary name (e.g., "helm", "sops").
	Name string

	// Description explains why this tool is needed.
	Description string
}

// ToolProvider is implemented by addons that require external CLI tools.
type ToolProvider interface {
	// RequiredTools returns the list of tools this addon needs.
	RequiredTools() []ToolSpec
}

// WizardField describes a single configuration field for the TUI wizard.
type WizardField struct {
	// Key is the settings map key (e.g., "provider", "repository").
	Key string

	// Label is displayed to the user (e.g., "GitOps Provider").
	Label string

	// Default is the pre-filled value.
	Default string

	// Help is the hint text shown below the input.
	Help string

	// Required marks the field as mandatory when the addon is enabled.
	Required bool
}

// WizardProvider is implemented by addons that contribute fields to the TUI wizard.
type WizardProvider interface {
	// WizardFields returns the fields this addon contributes to the wizard.
	WizardFields() []WizardField
}
