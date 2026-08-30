package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// StartSpinner renders a live spinner to stderr during a long operation,
// returning a no-op stop when logutil.ProgressBarsEnabled is false. stop
// clears the line and blocks until the goroutine exits — call it before
// printing output that must appear below the spinner.
func StartSpinner(ctx context.Context, desc string) func() {
	if !logutil.ProgressBarsEnabled() {
		return func() {}
	}
	return startSpinner(ctx, desc, os.Stderr)
}

// StartStatusLine renders a spinner whose description can be replaced while
// running (live operator/CSR counts); returns set/stop funcs sharing
// StartSpinner's teardown semantics, both no-op when ProgressBarsEnabled is
// false.
func StartStatusLine(ctx context.Context, desc string) (set func(string), stop func()) {
	if !logutil.ProgressBarsEnabled() {
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
		// LIFO teardown: release runs before done closes, so <-done sees a
		// cleared line.
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

// clearLine implements lineOwner; the caller holds the line lock.
func (s *spinner) clearLine() {
	_, _ = fmt.Fprint(s.w, "\r\x1b[2K")
}

// setDesc replaces desc under the line lock so a repaint never reads a
// half-written value; shows on the next tick.
func (s *spinner) setDesc(d string) {
	lineReg.paint(s, func() { s.desc = d })
}

func (s *spinner) paint() {
	lineReg.paint(s, func() {
		elapsed := time.Since(s.start).Round(time.Second)
		frame := SpinnerStyle.Render(spinnerFrames[s.frame%len(spinnerFrames)])
		_, _ = fmt.Fprintf(s.w, "\r\x1b[2K%s %s (%s)", frame, s.desc, elapsed)
		s.frame++
	})
}
