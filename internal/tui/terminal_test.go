package tui

import "testing"

func TestTerminalWidthHonoursColumns(t *testing.T) {
	t.Setenv("COLUMNS", "72")
	SetTerminalWidth(0)
	t.Cleanup(func() { SetTerminalWidth(0) })

	if got := TerminalWidth(); got != 72 {
		t.Fatalf("TerminalWidth() = %d, want 72", got)
	}

	t.Setenv("COLUMNS", "abc")
	SetTerminalWidth(0)

	if got := TerminalWidth(); got < terminalWidthFloor {
		t.Fatalf("TerminalWidth() = %d, want detection or fallback (>= %d)", got, terminalWidthFloor)
	}
}

func TestSetTerminalWidthOverride(t *testing.T) {
	SetTerminalWidth(200)
	t.Cleanup(func() { SetTerminalWidth(0) })

	if got := TerminalWidth(); got != 200 {
		t.Fatalf("TerminalWidth() = %d, want 200", got)
	}

	SetTerminalWidth(10)
	if got := TerminalWidth(); got != 10 {
		t.Fatalf("TerminalWidth() = %d, want 10 (SetTerminalWidth bypasses the floor)", got)
	}
}
