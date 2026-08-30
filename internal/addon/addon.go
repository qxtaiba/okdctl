// Package addon provides a plugin system for optional cluster features.
// Addons self-register via init() and are resolved in dependency order.
package addon

import (
	"context"
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// Addon is the contract every pluggable cluster feature satisfies;
// implementations self-register via init() and install in Manager-determined
// dependency order.
type Addon interface {
	Info() Metadata

	Install(ctx context.Context, env *Environment) error
	Verify(ctx context.Context, env *Environment) error
	Uninstall(ctx context.Context, env *Environment) error
}

// Metadata is the addon descriptor returned from Info(); Name doubles as the
// config key, Priority sets install order (lower first), and Dependencies
// lists required addons.
type Metadata struct {
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
	// ValidateSettings returns human-readable validation errors for settings
	// (empty means valid); Manager.installAndVerify runs it before Install
	// and aborts on any error.
	ValidateSettings(settings map[string]string) []string
}

// ToolSpec names an external binary an addon requires and why.
type ToolSpec struct {
	Name        string
	Description string
}

// ToolProvider is implemented by addons that require specific external host
// tools, surfaced by the installer before install begins.
type ToolProvider interface {
	RequiredTools() []ToolSpec
}
