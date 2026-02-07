// Package download provides utilities for downloading files with
// checksum verification and archive extraction.
package download

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Progress display constants
const (
	// ProgressUpdateInterval is the minimum time between progress bar updates.
	ProgressUpdateInterval = 100 * time.Millisecond

	// ProgressBarWidth is the width of the progress bar in characters.
	ProgressBarWidth = 30
)

// progressWriter wraps an io.Writer to track and display download progress.
type progressWriter struct {
	writer      Writer
	total       int64
	written     int64          // accessed atomically
	lastPercent int32          // accessed atomically
	stopped     int32          // accessed atomically - stops all output when set
	lastUpdate  atomic.Value   // stores time.Time atomically
	mu          sync.Mutex     // protects printProgress
}

// Writer is a minimal interface for the underlying writer.
type Writer interface {
	Write(p []byte) (n int, err error)
}

// stop prevents any further progress output (used when cancelled/errored).
func (pw *progressWriter) stop() {
	atomic.StoreInt32(&pw.stopped, 1)
}

// isStopped returns true if progress output has been stopped.
func (pw *progressWriter) isStopped() bool {
	return atomic.LoadInt32(&pw.stopped) != 0
}

// Write implements io.Writer and updates progress.
func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if err != nil {
		return n, err
	}

	atomic.AddInt64(&pw.written, int64(n))

	if pw.total > 0 && !pw.isStopped() {
		// Only update at ProgressUpdateInterval to avoid too much output
		lastUpdate := pw.lastUpdate.Load()
		if lastUpdate == nil || time.Since(lastUpdate.(time.Time)) > ProgressUpdateInterval {
			pw.lastUpdate.Store(time.Now())
			written := atomic.LoadInt64(&pw.written)
			percent := int32(float64(written) / float64(pw.total) * 100)
			if percent != atomic.LoadInt32(&pw.lastPercent) {
				atomic.StoreInt32(&pw.lastPercent, percent)
				pw.mu.Lock()
				pw.printProgress()
				pw.mu.Unlock()
			}
		}
	}

	return n, nil
}

// printProgress displays the current download progress.
func (pw *progressWriter) printProgress() {
	written := atomic.LoadInt64(&pw.written)
	writtenMB := float64(written) / 1024 / 1024
	totalMB := float64(pw.total) / 1024 / 1024

	percent := atomic.LoadInt32(&pw.lastPercent)
	filled := int(float64(percent) / 100 * float64(ProgressBarWidth))
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", ProgressBarWidth-filled)

	// Print on same line (carriage return)
	fmt.Printf("\r[INFO] download: [%s] %3d%% (%.1f/%.1f MB)", bar, percent, writtenMB, totalMB)
}

// finish prints the final progress line with newline.
// Does nothing if stopped (e.g., due to cancellation).
func (pw *progressWriter) finish() {
	if pw.total > 0 && !pw.isStopped() {
		atomic.StoreInt32(&pw.lastPercent, 100)
		pw.mu.Lock()
		pw.printProgress()
		pw.mu.Unlock()
		fmt.Print("\n") // Final newline after progress bar
	}
}
