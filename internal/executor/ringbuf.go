package executor

import "strings"

// maxCapturedLines is the ring depth for the stdout/stderr tails in Result;
// earlier lines are discarded once full.
const maxCapturedLines = 200

// maxPartial caps the pending partial line so a child that never emits a
// newline cannot grow it without bound; longer runs are emitted in chunks.
const maxPartial = 64 * 1024

// lineWriter splits a byte stream into lines for emit; not thread-safe, but
// os/exec writes each pipe from a single goroutine so no locking is needed.
type lineWriter struct {
	partial string
	emit    func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	s := w.partial + string(p)
	for {
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		w.emit(s[:idx])
		s = s[idx+1:]
	}
	for len(s) > maxPartial {
		w.emit(s[:maxPartial])
		s = s[maxPartial:]
	}
	w.partial = s
	return len(p), nil
}

// flush emits the line a child left without a trailing newline; call it only
// once the pipe is drained.
func (w *lineWriter) flush() {
	if w.partial != "" {
		w.emit(w.partial)
		w.partial = ""
	}
}

// ring retains only the last max lines pushed.
type ring struct {
	lines []string
	max   int
	pos   int
	full  bool
	// dropped is true once a line is actually overwritten; full alone
	// doesn't mean anything was lost yet.
	dropped bool
}

func newRing(maxLines int) *ring {
	return &ring{lines: make([]string, maxLines), max: maxLines}
}

func (r *ring) push(line string) {
	if r.full {
		r.dropped = true
	}
	r.lines[r.pos] = line
	r.pos = (r.pos + 1) % r.max
	if r.pos == 0 {
		r.full = true
	}
}

// tail returns the retained lines in chronological order, joined by newlines.
func (r *ring) tail() string {
	if !r.full {
		return strings.Join(r.lines[:r.pos], "\n")
	}
	parts := make([]string, 0, r.max)
	for i := range r.max {
		parts = append(parts, r.lines[(r.pos+i)%r.max])
	}
	return strings.Join(parts, "\n")
}
