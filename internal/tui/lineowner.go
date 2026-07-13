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

// deregister removes o as owner without clearing its line. The checklist uses
// it after committing a step's final line with a trailing newline: the line is
// already permanent, so a clearLine here would erase the fresh empty line
// below it. No-op if o is not the current owner.
func (r *lineRegistry) deregister(o lineOwner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner == o {
		r.owner = nil
		r.active.Store(false)
	}
}

// paint runs o's repaint under the line lock, but only while o is still the
// active owner. The guard mirrors release's owner check: with two concurrent
// owners handing the line back and forth (the step checklist and the
// spinner/status line it spawns per step), a stale owner whose line was already
// replaced must not paint over the current owner's line.
func (r *lineRegistry) paint(o lineOwner, fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner == o {
		fn()
	}
}

// withLine erases the active owner's line, then runs fn, under the line
// lock. Always locks, even when active is false: a lock-free fast path here
// let a Handle() call that sampled active just before a concurrent register
// skip the lock while another, already in-flight Handle() call (still on the
// pre-transition value) wrote straight through — same io.Writer, racing
// under two different locks. r.mu is uncontended on the common no-owner
// path, so the cost is one extra lock/unlock per log record.
func (r *lineRegistry) withLine(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner != nil {
		r.owner.clearLine()
	}
	fn()
}
