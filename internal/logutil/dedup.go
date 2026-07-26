package logutil

import "log/slog"

// DedupWarner gates repeated failure logging in poll/retry loops: the first
// occurrence of a key (typically err.Error()) logs at Warn, identical
// consecutive keys demote to Debug with a " (repeated)" message suffix, and
// a changed key logs at Warn again. Call Reset on a clean tick so the next
// failure re-warns. Not safe for concurrent use; scope one per loop.
type DedupWarner struct {
	log  *slog.Logger
	last string
}

// NewDedupWarner returns a DedupWarner logging through l (NopLogger when nil).
func NewDedupWarner(l *slog.Logger) *DedupWarner {
	return &DedupWarner{log: OrNop(l)}
}

// Warn logs msg with args at Warn when key differs from the previous call's
// key, or at Debug with a " (repeated)" suffix when it is identical.
func (d *DedupWarner) Warn(key, msg string, args ...any) {
	if key != d.last {
		d.log.Warn(msg, args...)
		d.last = key
		return
	}
	d.log.Debug(msg+" (repeated)", args...)
}

// Reset re-arms the gate so the next Warn logs at Warn level again.
func (d *DedupWarner) Reset() { d.last = "" }
