package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestBoxedSectionIsRequestedWidth(t *testing.T) {
	SetTerminalWidth(200)
	t.Cleanup(func() { SetTerminalWidth(0) })

	out := BoxedSectionCompact("first line\nsecond line", "t", 90)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w != 90 {
			t.Errorf("line %d cols wide, want exactly 90: %q", w, line)
		}
	}
}

func TestBoxedSectionNeverWiderThanTerminal(t *testing.T) {
	t.Cleanup(func() { SetTerminalWidth(0) })

	for termWidth := 40; termWidth <= 200; termWidth += 7 {
		SetTerminalWidth(termWidth)
		for _, contentWidth := range []int{10, 88, 150, 300} {
			content := strings.Repeat("x", contentWidth)
			out := BoxedSectionCompact(content, "t", DefaultBoxWidth)
			for _, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w > termWidth {
					t.Errorf("term=%d content=%d: line %d cols wide, want <= %d: %q",
						termWidth, contentWidth, w, termWidth, line)
				}
			}
		}
	}
}

func TestBoxedSectionTruncatesOverwideLine(t *testing.T) {
	SetTerminalWidth(200)
	t.Cleanup(func() { SetTerminalWidth(0) })

	long := strings.Repeat("a", 300)
	out := BoxedSectionCompact(long, "t", 60)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line %d cols wide, want <= 60: %q", w, line)
		}
	}
	if !strings.Contains(out, "…") {
		t.Errorf("overwide raw line should be truncated with an ellipsis:\n%s", out)
	}
}
