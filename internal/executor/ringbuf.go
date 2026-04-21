package executor

import "strings"

// constMaxLines is the ring-buffer depth for stdout and stderr capture on
// the default Run path. Callers receive the last constMaxLines lines of
// output; earlier output is discarded once the buffer is full.
const constMaxLines = 200

// ringWriter is an io.Writer that retains only the last max lines written
// to it. It is NOT thread-safe; os/exec writes to cmd.Stdout and cmd.Stderr
// from a single goroutine per pipe, so no locking is needed for a single
// Run call.
type ringWriter struct {
	lines []string
	max   int
	pos   int
	full  bool
	// partial holds an incomplete line (no trailing newline yet).
	partial string
}

func newRingWriter(maxLines int) *ringWriter {
	return &ringWriter{lines: make([]string, maxLines), max: maxLines}
}

func (r *ringWriter) Write(p []byte) (int, error) {
	n := len(p)
	s := r.partial + string(p)
	r.partial = ""

	for {
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			r.partial = s
			break
		}
		r.push(s[:idx])
		s = s[idx+1:]
	}
	return n, nil
}

func (r *ringWriter) push(line string) {
	r.lines[r.pos] = line
	r.pos = (r.pos + 1) % r.max
	if r.pos == 0 {
		r.full = true
	}
}

// tail returns the retained lines in chronological order joined by newlines.
// Any partial line (no trailing newline) is appended as the final entry.
func (r *ringWriter) tail() string {
	var parts []string
	if r.full {
		parts = make([]string, 0, r.max)
		for i := 0; i < r.max; i++ {
			parts = append(parts, r.lines[(r.pos+i)%r.max])
		}
	} else {
		parts = make([]string, 0, r.pos)
		parts = append(parts, r.lines[:r.pos]...)
	}
	if r.partial != "" {
		parts = append(parts, r.partial)
	}
	return strings.Join(parts, "\n")
}
