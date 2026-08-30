package addon

import (
	"fmt"
	"slices"
	"sync"

	"github.com/qxtaiba/okdctl/internal/config"
)

// registry is the global addon registry, populated via init() in each addon package.
var registry = &addonRegistry{
	addons: make(map[string]Addon),
}

// addonRegistry holds addons in Register-call (insertion) order, safe for
// concurrent access.
type addonRegistry struct {
	mu     sync.RWMutex
	addons map[string]Addon
	order  []string // insertion order for deterministic iteration
}

// Register adds an addon to the global registry, erroring if the name is
// already taken. Callers in init() must handle the error explicitly
// (typically by panicking), since init cannot propagate one.
func Register(a Addon) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	name := a.Info().Name
	if _, exists := registry.addons[name]; exists {
		return fmt.Errorf("addon %q already registered", name)
	}
	registry.addons[name] = a
	registry.order = append(registry.order, name)
	return nil
}

// Get returns the addon registered under name, or nil if no such addon exists.
func Get(name string) Addon {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.addons[name]
}

// All returns every registered addon in insertion order (not alphabetical).
func All() []Addon {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	result := make([]Addon, 0, len(registry.order))
	for _, name := range registry.order {
		result = append(result, registry.addons[name])
	}
	return result
}

// Enabled returns the subset of registered addons whose config entry has
// Enabled=true, preserving insertion order.
func Enabled(cfg *config.Config) []Addon {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	var result []Addon
	for _, name := range registry.order {
		ac, ok := cfg.Addons[name]
		if ok && ac.Enabled {
			result = append(result, registry.addons[name])
		}
	}
	return result
}

// Names returns the registered addon names in insertion order.
func Names() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	return slices.Clone(registry.order)
}

// IsRegistered reports whether name is in the registry; unused today, kept
// for a future addon-validate verb.
func IsRegistered(name string) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	_, ok := registry.addons[name]
	return ok
}
