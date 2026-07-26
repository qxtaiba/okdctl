package distribution

// PhaseContext provides type-safe data sharing between steps within a
// phase. Today postinstall is the only consumer; the type is scaffolded
// for symmetric use as resume-checkpoint work lands in other phases.
// Orchestrator.Run executes steps serially, so access is single-threaded.
// Must be created via NewPhaseContext; the zero value panics on use.
type PhaseContext[T any] struct {
	data        T
	initialized bool
}

// NewPhaseContext returns a PhaseContext seeded with initial.
func NewPhaseContext[T any](initial T) *PhaseContext[T] {
	return &PhaseContext[T]{data: initial, initialized: true}
}

// Get returns a copy of the stored value.
func (c *PhaseContext[T]) Get() T {
	if !c.initialized {
		panic("PhaseContext: must be created via NewPhaseContext")
	}
	return c.data
}

// Update calls fn with a pointer to the stored data.
func (c *PhaseContext[T]) Update(fn func(*T)) {
	if !c.initialized {
		panic("PhaseContext: must be created via NewPhaseContext")
	}
	fn(&c.data)
}
