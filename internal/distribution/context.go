package distribution

// PhaseContext shares typed data between phase steps; the zero value panics,
// so create it via NewPhaseContext.
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
