package tui

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/term"
)

// DefaultTerminalWidth is the terminal width used when it cannot be detected.
const DefaultTerminalWidth = 100

// terminalWidthFloor is the minimum width TerminalWidth ever reports, even
// when COLUMNS or term.GetSize returns something smaller.
const terminalWidthFloor = 40

var (
	terminalWidthOnce     sync.Once
	terminalWidthDetected int
	terminalWidthOverride atomic.Int64 // 0 means "use detection"
)

// TerminalWidth reports the terminal width — COLUMNS when it parses as a
// positive integer, else term.GetSize on stdout, else on stderr, else
// DefaultTerminalWidth, floored at 40 — detecting once per process unless
// SetTerminalWidth rearms it.
func TerminalWidth() int {
	if w := terminalWidthOverride.Load(); w != 0 {
		return int(w)
	}
	terminalWidthOnce.Do(func() {
		terminalWidthDetected = floorWidth(detectTerminalWidth())
	})
	return terminalWidthDetected
}

// SetTerminalWidth overrides TerminalWidth for tests and callers that already
// know the width; SetTerminalWidth(0) restores detection, re-arming it so the
// next call re-detects.
func SetTerminalWidth(w int) {
	if w == 0 {
		terminalWidthOnce = sync.Once{}
		terminalWidthOverride.Store(0)
		return
	}
	terminalWidthOverride.Store(int64(w))
}

func detectTerminalWidth() int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		return w
	}
	if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil {
		return w
	}
	return DefaultTerminalWidth
}

func floorWidth(w int) int {
	if w < terminalWidthFloor {
		return terminalWidthFloor
	}
	return w
}
