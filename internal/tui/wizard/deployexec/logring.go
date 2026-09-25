package deployexec

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LogRingCap is how many human log lines the pane keeps: enough to read a
// step's chatter back, small enough to snapshot on every frame.
const LogRingCap = 500

// LogLine is one captured human log line: when it was logged, its level, and
// the message with its structured fields appended.
type LogLine struct {
	At    time.Time
	Level string
	Text  string
}

// LogSource is what the log pane reads: the buffered lines plus the absolute
// stream index of the first, so a locked window can name a point the ring may
// since have dropped.
type LogSource interface {
	Snapshot() (lines []LogLine, first int64)
}

// LogRing keeps the last cap human log lines for the deploy stream's log pane.
// Writes arrive on whichever goroutine logged them and reads happen on the
// render loop, so every access takes the ring's own lock.
type LogRing struct {
	cap int

	mu    sync.Mutex
	buf   []LogLine
	first int64 // absolute stream index of buf[0]
}

// NewLogRing returns an empty ring holding at most capacity lines; a
// non-positive capacity falls back to LogRingCap.
func NewLogRing(capacity int) *LogRing {
	if capacity <= 0 {
		capacity = LogRingCap
	}
	return &LogRing{cap: capacity, buf: make([]LogLine, 0, capacity)}
}

// Handler returns the slog handler that tees every record into the ring and
// forwards it to next; a nil next makes the ring the only sink.
func (r *LogRing) Handler(next slog.Handler) slog.Handler {
	return &ringHandler{ring: r, next: next}
}

// Snapshot copies the buffered lines out with the absolute stream index of the
// first, so the caller renders off a value the writers cannot mutate.
func (r *LogRing) Snapshot() (lines []LogLine, first int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LogLine, len(r.buf))
	copy(out, r.buf)
	return out, r.first
}

// append adds line, evicting the oldest once the ring is full.
func (r *LogRing) append(line LogLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, line)
		return
	}
	copy(r.buf, r.buf[1:])
	r.buf[len(r.buf)-1] = line
	r.first++
}

// ringHandler tees each record into a LogRing before forwarding it. Attrs
// gathered through WithAttrs are rendered into the ring's text and handed to the
// wrapped handler, so neither sink loses them.
type ringHandler struct {
	ring  *LogRing
	next  slog.Handler
	attrs []slog.Attr
}

// Enabled reports whether the wrapped handler wants lvl; a ring with no handler
// behind it accepts everything, since the pane is then the only reader.
func (h *ringHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	if h.next == nil {
		return true
	}
	return h.next.Enabled(ctx, lvl)
}

// Handle appends the record's human line to the ring, then forwards it.
func (h *ringHandler) Handle(ctx context.Context, rec slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler passes slog.Record by value
	h.ring.append(LogLine{At: rec.Time, Level: rec.Level.String(), Text: recordText(&rec, h.attrs)})
	if h.next == nil {
		return nil
	}
	return h.next.Handle(ctx, rec)
}

// WithAttrs carries attrs into both sinks.
func (h *ringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := &ringHandler{ring: h.ring, attrs: append(append([]slog.Attr{}, h.attrs...), attrs...)}
	if h.next != nil {
		clone.next = h.next.WithAttrs(attrs)
	}
	return clone
}

// WithGroup forwards the group to the wrapped handler; the ring's flat text
// renders grouped attrs under their own keys.
func (h *ringHandler) WithGroup(name string) slog.Handler {
	clone := &ringHandler{ring: h.ring, attrs: h.attrs}
	if h.next != nil {
		clone.next = h.next.WithGroup(name)
	}
	return clone
}

// recordText renders rec as the one line the pane shows: the message, then every
// attr as key=value in the order they were logged.
func recordText(rec *slog.Record, carried []slog.Attr) string {
	var b strings.Builder
	b.WriteString(rec.Message)
	for _, a := range carried {
		writeAttr(&b, a)
	}
	rec.Attrs(func(a slog.Attr) bool {
		writeAttr(&b, a)
		return true
	})
	return b.String()
}

func writeAttr(b *strings.Builder, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	fmt.Fprintf(b, " %s=%v", a.Key, a.Value)
}
