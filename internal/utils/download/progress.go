package download

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

const (
	ProgressUpdateInterval = 100 * time.Millisecond
	ProgressBarWidth       = 30
)

type progressWriter struct {
	writer      Writer
	total       int64
	written     int64        // accessed atomically
	lastPercent int32        // accessed atomically
	stopped     int32        // accessed atomically - stops all output when set
	lastUpdate  atomic.Value // stores time.Time atomically
	mu          sync.Mutex   // protects printProgress
	isTTY       bool
}

var stdoutIsTTY = term.IsTerminal(int(os.Stdout.Fd()))

type Writer interface {
	Write(p []byte) (n int, err error)
}

func (pw *progressWriter) stop() {
	atomic.StoreInt32(&pw.stopped, 1)
}

func (pw *progressWriter) isStopped() bool {
	return atomic.LoadInt32(&pw.stopped) != 0
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if err != nil {
		return n, err
	}

	atomic.AddInt64(&pw.written, int64(n))

	if pw.total > 0 && !pw.isStopped() {
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

func (pw *progressWriter) printProgress() {
	if !pw.isTTY {
		return
	}

	written := atomic.LoadInt64(&pw.written)
	writtenMB := float64(written) / 1024 / 1024
	totalMB := float64(pw.total) / 1024 / 1024

	percent := atomic.LoadInt32(&pw.lastPercent)
	filled := int(float64(percent) / 100 * float64(ProgressBarWidth))
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", ProgressBarWidth-filled)

	fmt.Printf("\r[INFO] download: [%s] %3d%% (%.1f/%.1f MB)", bar, percent, writtenMB, totalMB)
}

func (pw *progressWriter) finish() {
	if pw.total > 0 && !pw.isStopped() {
		atomic.StoreInt32(&pw.lastPercent, 100)
		pw.mu.Lock()
		pw.printProgress()
		pw.mu.Unlock()
		if pw.isTTY {
			fmt.Print("\n")
		}
	}
}
