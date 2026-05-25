package distribution

import "sync"

// PhaseContext provides type-safe, concurrency-safe data sharing between
// steps within a phase. Today postinstall is the only consumer; the type
// is scaffolded for symmetric use as resume-checkpoint work lands in other
// phases (api:dd75bdeb; roadmap state:4f69fc9d, state:262af6e4). The RWMutex is
// forward-looking: Orchestrator.Run is serial today, but a parallel-step
// mode would need concurrent Get/Update without a retrofit here.
// Must be created via NewPhaseContext; the zero value panics on use.
type PhaseContext[T any] struct {
	mu          sync.RWMutex
	data        T
	initialized bool
}

// NewPhaseContext returns a PhaseContext seeded with initial.
func NewPhaseContext[T any](initial T) *PhaseContext[T] {
	return &PhaseContext[T]{data: initial, initialized: true}
}

// Get returns a copy of the stored value under the read lock.
func (c *PhaseContext[T]) Get() T {
	if !c.initialized {
		panic("PhaseContext: must be created via NewPhaseContext")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// Update calls fn with a pointer to the stored data while holding the write
// lock. fn must not call Get or Update on the same PhaseContext —
// sync.RWMutex is not reentrant and doing so deadlocks.
func (c *PhaseContext[T]) Update(fn func(*T)) {
	if !c.initialized {
		panic("PhaseContext: must be created via NewPhaseContext")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(&c.data)
}
