package addon

import (
	"fmt"
	"sync"

	"github.com/qxtaiba/okdctl/internal/config"
)

// registry is the global addon registry, populated via init() in each addon package.
var registry = &Registry{
	addons: make(map[string]Addon),
}

// Registry holds the set of addons an okdctl build knows about. Lookups are
// safe for concurrent use; iteration order is insertion order (the order the
// addon packages' init() functions called Register), not alphabetical.
type Registry struct {
	mu     sync.RWMutex
	addons map[string]Addon
	order  []string // insertion order for deterministic iteration
}

// Register adds an addon to the global registry. It returns an error if an
// addon with the same name is already registered. Callers invoking Register
// from an init() function should handle the error explicitly (typically by
// panicking — init() cannot propagate errors).
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

	out := make([]string, len(registry.order))
	copy(out, registry.order)
	return out
}

// IsRegistered reports whether name is in the registry. Symmetric with
// Get/Names/All; currently no caller, retained as the canonical predicate
// for the future "okdctl addon validate" verb and wizard pre-checks.
func IsRegistered(name string) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	_, ok := registry.addons[name]
	return ok
}
