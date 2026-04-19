package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var spinnerFrames = []string{"|", "/", "-", "\\"}

// StartSpinner renders a live spinner line to stderr during a long-running
// operation. Returns a no-op stop function when ProgressBarsEnabled is false
// (non-TTY stderr or JSON log format). The returned stop function clears the
// spinner line and blocks until the rendering goroutine has exited — call it
// before printing any output that must appear below the spinner position.
func StartSpinner(ctx context.Context, desc string) func() {
	if !ProgressBarsEnabled() {
		return func() {}
	}

	done := make(chan struct{})
	stopCh := make(chan struct{})
	stop := sync.OnceFunc(func() { close(stopCh) })

	go func() {
		defer close(done)
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		start := time.Now()
		frame := 0
		for {
			select {
			case <-stopCh:
				spinnerClearLine(desc)
				return
			case <-ctx.Done():
				spinnerClearLine(desc)
				return
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				line := fmt.Sprintf("\r%s %s (%s)",
					spinnerFrames[frame%len(spinnerFrames)], desc, elapsed)
				_, _ = fmt.Fprint(os.Stderr, line)
				frame++
			}
		}
	}()

	return func() {
		stop()
		<-done
	}
}

func spinnerClearLine(desc string) {
	width := 4 + len(desc) + 16
	_, _ = fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", width))
}
