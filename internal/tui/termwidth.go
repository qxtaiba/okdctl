package tui

import (
	"os"
	"sync/atomic"

	"golang.org/x/term"
)

// terminalWidthOverride, when nonzero, is the width TerminalWidth reports
// in place of the OS-detected terminal width — the process's real
// controlling terminal (a CI pipe, or the sandbox running a test) reports
// whatever it reports, so a headless render that must match a chosen
// scenario width needs an escape hatch.
var terminalWidthOverride atomic.Int32

// SetTerminalWidth overrides the width TerminalWidth reports for golden
// tests — no production caller sets it — and a 0 clears the override to
// resume OS detection.
func SetTerminalWidth(w int) {
	terminalWidthOverride.Store(int32(w)) //nolint:gosec // G115: w is a terminal column count, always far below int32's range
}

// TerminalWidth returns the SetTerminalWidth override if one is set, else
// the OS-reported width of stdout, else fallback when detection fails
// (stdout is a pipe, not a terminal).
func TerminalWidth(fallback int) int {
	if w := int(terminalWidthOverride.Load()); w > 0 {
		return w
	}
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return fallback
	}
	return w
}
