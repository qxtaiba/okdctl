package tui

import (
	"sync"
	"sync/atomic"
)

// lineOwner is a writer that owns the current terminal line — a spinner or,
// later, a step-checklist renderer. The stderr log handler asks the active
// owner to erase its painted line before writing a record so log lines start
// at column 0; the owner repaints on its next tick.
type lineOwner interface {
	// clearLine erases the owner's currently painted line. The caller holds
	// the line lock, so implementations must not re-acquire it.
	clearLine()
}

// lineReg coordinates the single active line owner with the stderr log
// handler. Both the owner's repaint and a record write take reg.mu, so a
// record is never interleaved with a half-painted spinner line.
var lineReg lineRegistry

type lineRegistry struct {
	mu     sync.Mutex
	owner  lineOwner
	active atomic.Bool
}

// register installs o as the active line owner.
func (r *lineRegistry) register(o lineOwner) {
	r.mu.Lock()
	r.owner = o
	r.active.Store(true)
	r.mu.Unlock()
}

// release clears o's line and removes it as owner. No-op if o is not the
// current owner (a later owner already replaced it).
func (r *lineRegistry) release(o lineOwner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner == o {
		o.clearLine()
		r.owner = nil
		r.active.Store(false)
	}
}

// paint runs the owner's repaint under the line lock.
func (r *lineRegistry) paint(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn()
}

// withLine erases the active owner's line, then runs fn, under the line lock.
// The lock-free fast path keeps the no-spinner logging hot path (including
// every non-TTY run) free of mutex traffic: no owner ever registers there.
func (r *lineRegistry) withLine(fn func()) {
	if !r.active.Load() {
		fn()
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner != nil {
		r.owner.clearLine()
	}
	fn()
}
