package addon

import (
	"fmt"
	"sync"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
)

// registry is the global addon registry, populated via init() in each addon package.
var registry = &Registry{
	addons: make(map[string]Addon),
}

// Registry holds all registered addons.
type Registry struct {
	mu     sync.RWMutex
	addons map[string]Addon
	order  []string // insertion order for deterministic iteration
}

// Register adds an addon to the global registry.
// Called from each addon package's init() function.
// Panics if an addon with the same name is already registered.
func Register(a Addon) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	name := a.Info().Name
	if _, exists := registry.addons[name]; exists {
		panic(fmt.Sprintf("addon %q already registered", name))
	}
	registry.addons[name] = a
	registry.order = append(registry.order, name)
}

// Get returns a registered addon by name, or nil if not found.
func Get(name string) Addon {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.addons[name]
}

// All returns all registered addons in registration order.
func All() []Addon {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	result := make([]Addon, 0, len(registry.order))
	for _, name := range registry.order {
		result = append(result, registry.addons[name])
	}
	return result
}

// Enabled returns addons that are enabled in the given config.
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

// Names returns the names of all registered addons.
func Names() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	out := make([]string, len(registry.order))
	copy(out, registry.order)
	return out
}

// IsRegistered checks if an addon name is known.
func IsRegistered(name string) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	_, ok := registry.addons[name]
	return ok
}
