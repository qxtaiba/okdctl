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

func Get(name string) Addon {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.addons[name]
}

func All() []Addon {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	result := make([]Addon, 0, len(registry.order))
	for _, name := range registry.order {
		result = append(result, registry.addons[name])
	}
	return result
}

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

func Names() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	out := make([]string, len(registry.order))
	copy(out, registry.order)
	return out
}

func IsRegistered(name string) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	_, ok := registry.addons[name]
	return ok
}
