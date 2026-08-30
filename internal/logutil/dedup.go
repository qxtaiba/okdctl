package logutil

import "log/slog"

// DedupWarner gates repeated failure logging in poll/retry loops: the first
// occurrence of a key logs at Warn, repeats demote to Debug, and a changed
// key warns again. Not safe for concurrent use; scope one per loop.
type DedupWarner struct {
	log  *slog.Logger
	last string
}

// NewDedupWarner returns a DedupWarner logging through l (NopLogger when nil).
func NewDedupWarner(l *slog.Logger) *DedupWarner {
	return &DedupWarner{log: OrNop(l)}
}

// Warn logs at Warn when key differs from the previous call, or at Debug with a
// " (repeated)" suffix when identical.
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
