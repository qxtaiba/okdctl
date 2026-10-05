// Package logview is the shared log surface behind the deploy stream and
// the lifecycle execution screens: a bounded slog ring, the pane/tail/full
// window renderers, and the Surface state machine that gives every consumer
// the same lock/full-screen/paging vocabulary.
package logview

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// DefaultCap is how many human log lines the pane keeps: enough headroom to
// page back through a whole failed run's info-level chatter (a few hundred
// lines), while a per-frame Snapshot still copies only ~112KB of headers.
const DefaultCap = 2000

// Line is one captured human log line: when it was logged, its level, and
// the message with its structured fields appended.
type Line struct {
	At    time.Time
	Level string
	Text  string
}

// Source is what the log pane reads: the buffered lines plus the absolute
// stream index of the first, so a locked window can name a point the ring may
// since have dropped.
type Source interface {
	Snapshot() (lines []Line, first int64)
}

// Ring keeps the last cap human log lines for an exec screen's log pane.
// Writes arrive on whichever goroutine logged them and reads happen on the
// render loop, so every access takes the ring's own lock.
type Ring struct {
	cap int

	mu    sync.Mutex
	buf   []Line
	first int64 // absolute stream index of buf[0]
}

// NewRing returns an empty ring holding at most capacity lines; a
// non-positive capacity falls back to DefaultCap.
func NewRing(capacity int) *Ring {
	if capacity <= 0 {
		capacity = DefaultCap
	}
	return &Ring{cap: capacity, buf: make([]Line, 0, capacity)}
}

// Handler returns the slog handler that tees every record into the ring and
// forwards it to next; a nil next makes the ring the only sink.
func (r *Ring) Handler(next slog.Handler) slog.Handler {
	return &ringHandler{ring: r, next: next}
}

// Snapshot copies the buffered lines out with the absolute stream index of the
// first, so the caller renders off a value the writers cannot mutate.
func (r *Ring) Snapshot() (lines []Line, first int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Line, len(r.buf))
	copy(out, r.buf)
	return out, r.first
}

// Append adds line, evicting the oldest once the ring is full; production
// writes arrive through Handler's slog tee — Append is the seam scripted
// feeds and fixtures fill the ring through.
func (r *Ring) Append(line Line) {
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

// ringHandler tees each record into a Ring before forwarding it. Attrs
// gathered through WithAttrs are rendered into the ring's text and handed to the
// wrapped handler, so neither sink loses them.
type ringHandler struct {
	ring  *Ring
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
	h.ring.Append(Line{At: rec.Time, Level: rec.Level.String(), Text: recordText(&rec, h.attrs)})
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

// writeAttr formats a into the line body before it ever reaches the pane's
// render sites, sanitizing the formatted value since an attr — "err" above
// all — routinely carries backend/subprocess text this process never
// composed.
func writeAttr(b *strings.Builder, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	fmt.Fprintf(b, " %s=%s", a.Key, tui.SanitizeTerminalEscapes(a.Value.String()))
}
