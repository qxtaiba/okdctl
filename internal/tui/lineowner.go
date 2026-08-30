package tui

import "sync"

// lineOwner owns the current terminal line (spinner, step-checklist); the
// stderr handler clears it before each record so log lines start at column 0.
type lineOwner interface {
	// clearLine erases the owner's line; the caller holds the line lock, so
	// implementations must not re-acquire it.
	clearLine()
}

// lineReg coordinates the single active line owner with the stderr handler;
// both take reg.mu so a record never interleaves with a half-painted line.
var lineReg lineRegistry

type lineRegistry struct {
	mu    sync.Mutex
	owner lineOwner
}

// register installs o as owner, clearing the outgoing owner's line first so
// a shorter takeover leaves no stale tail.
func (r *lineRegistry) register(o lineOwner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner != nil && r.owner != o {
		r.owner.clearLine()
	}
	r.owner = o
}

// release clears o's line and removes it as owner; no-op if o is not the
// current owner.
func (r *lineRegistry) release(o lineOwner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner == o {
		o.clearLine()
		r.owner = nil
	}
}

// deregister removes o as owner without clearing its line (a step's final
// line already ends with a newline, so clearing would erase the line below
// it); no-op if o isn't owner.
func (r *lineRegistry) deregister(o lineOwner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner == o {
		r.owner = nil
	}
}

// paint runs o's repaint under the line lock, but only while o is still the
// active owner — a stale owner (replaced mid-handoff between checklist and
// spinner) must not paint over the current one.
func (r *lineRegistry) paint(o lineOwner, fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner == o {
		fn()
	}
}

// withLine erases the active owner's line, then runs fn, under the line
// lock — always locking even with no owner, since a lock-free fast path let
// a sampled "no owner" race a concurrent register and write through
// unlocked.
func (r *lineRegistry) withLine(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owner != nil {
		r.owner.clearLine()
	}
	fn()
}
