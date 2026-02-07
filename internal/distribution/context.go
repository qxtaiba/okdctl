// Package distribution provides step-based provisioning orchestration.
package distribution

import "sync"

// PhaseContext provides type-safe data sharing between steps within a phase.
// Steps capture a PhaseContext via closure and read/write data through it,
// eliminating direct step-to-step references and enabling better decoupling.
//
// Example usage:
//
//	type MyPhaseContext struct {
//	    ResultIP string
//	    Count    int
//	}
//
//	pctx := NewPhaseContext(MyPhaseContext{})
//	// Step 1 writes data
//	pctx.Update(func(c *MyPhaseContext) { c.ResultIP = "10.0.0.1" })
//	// Step 2 reads data
//	ip := pctx.Get().ResultIP
type PhaseContext[T any] struct {
	mu   sync.RWMutex
	data T
}

// NewPhaseContext creates a new PhaseContext with the given initial value.
func NewPhaseContext[T any](initial T) *PhaseContext[T] {
	return &PhaseContext[T]{data: initial}
}

// Get returns a copy of the current context data.
// Use this when you need to read data that was written by a previous step.
func (c *PhaseContext[T]) Get() T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// Update atomically modifies the context data using the provided function.
// The function receives a pointer to the data for in-place modification.
// Use this when a step needs to store results for later steps to consume.
func (c *PhaseContext[T]) Update(fn func(*T)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(&c.data)
}
