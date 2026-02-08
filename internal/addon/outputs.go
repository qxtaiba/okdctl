package addon

import (
	"fmt"
	"sync"
)

// OutputStore is written after installation and read by dependent addons.
type OutputStore struct {
	mu   sync.RWMutex
	data map[string]map[string]string // addon name → key → value
}

func NewOutputStore() *OutputStore {
	return &OutputStore{
		data: make(map[string]map[string]string),
	}
}

func (s *OutputStore) Set(addonName, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data[addonName] == nil {
		s.data[addonName] = make(map[string]string)
	}
	s.data[addonName][key] = value
}

// Get returns the value or empty string if not found.
func (s *OutputStore) Get(addonName, key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if m, ok := s.data[addonName]; ok {
		return m[key]
	}
	return ""
}

// GetAll returns a copy of all outputs for an addon to prevent mutation.
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

// GetRequired returns the value for the given addon and key, or an error if
// the addon has no outputs, the key is missing, or the value is empty.
func (s *OutputStore) GetRequired(addonName, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.data[addonName]
	if !ok {
		return "", fmt.Errorf("addon %q has no outputs", addonName)
	}
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("addon %q has no output %q", addonName, key)
	}
	if v == "" {
		return "", fmt.Errorf("addon %q output %q is empty", addonName, key)
	}
	return v, nil
}
