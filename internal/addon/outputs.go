package addon

import "sync"

// OutputStore provides thread-safe cross-addon data sharing.
// Addons write outputs after installation; dependent addons read them.
type OutputStore struct {
	mu   sync.RWMutex
	data map[string]map[string]string // addon name → key → value
}

// NewOutputStore creates an empty output store.
func NewOutputStore() *OutputStore {
	return &OutputStore{
		data: make(map[string]map[string]string),
	}
}

// Set stores a value for an addon.
func (s *OutputStore) Set(addonName, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data[addonName] == nil {
		s.data[addonName] = make(map[string]string)
	}
	s.data[addonName][key] = value
}

// Get retrieves a value from an addon's outputs.
// Returns empty string if not found.
func (s *OutputStore) Get(addonName, key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if m, ok := s.data[addonName]; ok {
		return m[key]
	}
	return ""
}

// GetAll returns all outputs for an addon (copy to prevent mutation).
func (s *OutputStore) GetAll(addonName string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.data[addonName]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
