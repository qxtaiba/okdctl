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

func startSpinner(ctx context.Context, desc string, w io.Writer) func() {
	done := make(chan struct{})
	stopCh := make(chan struct{})
	stop := sync.OnceFunc(func() { close(stopCh) })

	sp := &spinner{w: w, desc: desc, start: time.Now()}
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

func (s *spinner) paint() {
	lineReg.paint(func() {
		elapsed := time.Since(s.start).Round(time.Second)
		frame := SpinnerStyle.Render(spinnerFrames[s.frame%len(spinnerFrames)])
		_, _ = fmt.Fprintf(s.w, "\r%s %s (%s)", frame, s.desc, elapsed)
		s.frame++
	})
}
