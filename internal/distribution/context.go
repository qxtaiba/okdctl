// Package distribution provides step-based provisioning orchestration.
package distribution

import "sync"

// PhaseContext provides type-safe data sharing between steps within a phase.
// Steps capture a PhaseContext via closure and read/write data through it,
// eliminating direct step-to-step references and enabling better decoupling.
type PhaseContext[T any] struct {
	mu   sync.RWMutex
	data T
}

func NewPhaseContext[T any](initial T) *PhaseContext[T] {
	return &PhaseContext[T]{data: initial}
}

func (c *PhaseContext[T]) Get() T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

func (c *PhaseContext[T]) Update(fn func(*T)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(&c.data)
}
