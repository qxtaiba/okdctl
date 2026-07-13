package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// StartSpinner renders a live spinner line to stderr during a long-running
// operation. Returns a no-op stop function when ProgressBarsEnabled is false
// (non-TTY stderr or JSON log format). The returned stop function clears the
// spinner line and blocks until the rendering goroutine has exited — call it
// before printing any output that must appear below the spinner position.
//
// While the spinner is active it registers as the line owner (see
// lineowner.go): the stderr log handler erases the spinner line before every
// record, so log lines never land on a half-painted frame; the spinner
// repaints on its next tick.
//
// Dual-stop-signal pattern: stopCh (sync.OnceFunc) plus ctx.Done() with a
// done channel for ordered teardown; preserve as-is.
func StartSpinner(ctx context.Context, desc string) func() {
	if !ProgressBarsEnabled() {
		return func() {}
	}
	return startSpinner(ctx, desc, os.Stderr)
}

// StartStatusLine renders a spinner whose description can be replaced in place
// while it runs — the install monitor uses it to surface live cluster-operator
// and CSR counts on one owned line. Returns a set func to update the detail
// and a stop func with StartSpinner's teardown semantics. Both are no-ops when
// ProgressBarsEnabled is false (non-TTY or JSON), so callers invoke them
// unconditionally.
func StartStatusLine(ctx context.Context, desc string) (set func(string), stop func()) {
	if !ProgressBarsEnabled() {
		return func(string) {}, func() {}
	}
	return startStatusLine(ctx, desc, os.Stderr)
}

func startStatusLine(ctx context.Context, desc string, w io.Writer) (set func(string), stop func()) {
	sp := &spinner{w: w, desc: desc, start: time.Now()}
	return sp.setDesc, runSpinner(ctx, sp)
}

func startSpinner(ctx context.Context, desc string, w io.Writer) func() {
	sp := &spinner{w: w, desc: desc, start: time.Now()}
	return runSpinner(ctx, sp)
}

func runSpinner(ctx context.Context, sp *spinner) func() {
	done := make(chan struct{})
	stopCh := make(chan struct{})
	stop := sync.OnceFunc(func() { close(stopCh) })

	lineReg.register(sp)

	go func() {
		// LIFO teardown: release (final clear + deregister) runs before done
		// closes, so a caller unblocked by <-done sees a cleared line.
		defer close(done)
		defer lineReg.release(sp)
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				sp.paint()
			}
		}
	}()

	return func() {
		stop()
		<-done
	}
}

type spinner struct {
	w     io.Writer
	desc  string
	start time.Time
	frame int
}

// clearLine implements lineOwner. The caller holds the line lock.
func (s *spinner) clearLine() {
	_, _ = fmt.Fprint(s.w, "\r\x1b[2K")
}

// setDesc replaces the spinner's description under the line lock so a repaint
// never reads a half-written desc; the change shows on the next tick.
func (s *spinner) setDesc(d string) {
	lineReg.paint(s, func() { s.desc = d })
}

func (s *spinner) paint() {
	lineReg.paint(s, func() {
		elapsed := time.Since(s.start).Round(time.Second)
		frame := SpinnerStyle.Render(spinnerFrames[s.frame%len(spinnerFrames)])
		_, _ = fmt.Fprintf(s.w, "\r%s %s (%s)", frame, s.desc, elapsed)
		s.frame++
	})
}
