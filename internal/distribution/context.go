package distribution

import "sync"

// PhaseContext provides type-safe data sharing between steps within a phase.
// Steps capture a PhaseContext via closure and read/write data through it,
// eliminating direct step-to-step references and enabling better decoupling.
type PhaseContext[T any] struct {
	mu   sync.RWMutex
	data T
}

// NewPhaseContext returns a PhaseContext seeded with initial.
func NewPhaseContext[T any](initial T) *PhaseContext[T] {
	return &PhaseContext[T]{data: initial}
}

// Get returns a copy of the stored value under the read lock.
func (c *PhaseContext[T]) Get() T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// Update calls fn with a pointer to the stored data while holding the
// write lock. fn must not call Get or Update on the same PhaseContext —
// sync.RWMutex is not reentrant and doing so will deadlock.
func (c *PhaseContext[T]) Update(fn func(*T)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(&c.data)
}
