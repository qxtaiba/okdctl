package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

func TestDottedKVWrapsUnderValueColumn(t *testing.T) {
	cases := []struct {
		name        string
		key         string
		keyColWidth int
		totalWidth  int
	}{
		{name: "normal key", key: "status", keyColWidth: DefaultKeyColWidth, totalWidth: 86},
		{name: "long key", key: "a-very-long-descriptive-key-name", keyColWidth: 10, totalWidth: 60},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := strings.Repeat("x", 120)
			out := DottedKeyValueFull(tc.key, value, tc.keyColWidth, tc.totalWidth)
			lines := strings.Split(tuitest.StripANSI(out), "\n")

			if len(lines) < 2 {
				t.Fatalf("got %d line(s), want > 1: %q", len(lines), out)
			}

			keyColWidth := tc.keyColWidth
			if keyColWidth <= 0 {
				keyColWidth = DefaultKeyColWidth
			}
			keyLen := lipgloss.Width(tc.key)
			dots := max(keyColWidth-keyLen-2, 3)
			valueStart := keyLen + 1 + dots + 1

			for i, line := range lines {
				if w := lipgloss.Width(line); w > tc.totalWidth {
					t.Errorf("line %d is %d cols, want <= %d: %q", i, w, tc.totalWidth, line)
				}
				if i > 0 && leadingSpaces(line) != valueStart {
					t.Errorf("continuation %d starts at column %d, want %d: %q", i, leadingSpaces(line), valueStart, line)
				}
			}
		})
	}
}

func TestDottedKVFallsBackToSingleLineWhenBudgetBelowFloor(t *testing.T) {
	const keyColWidth = 45
	const totalWidth = 53
	key := strings.Repeat("k", 43)
	value := strings.Repeat("x", 120)

	out := DottedKeyValueFull(key, value, keyColWidth, totalWidth)
	lines := strings.Split(tuitest.StripANSI(out), "\n")

	if len(lines) != 1 {
		t.Fatalf("got %d line(s), want a single-line fallback: %q", len(lines), out)
	}
	if w := lipgloss.Width(lines[0]); w > totalWidth {
		t.Errorf("fallback line is %d cols, want <= %d: %q", w, totalWidth, lines[0])
	}
}

func TestDottedKVNoWrapWhenUnbounded(t *testing.T) {
	value := strings.Repeat("y", 300)
	out := DottedKeyValueFull("key", value, DefaultKeyColWidth, 0)
	plain := tuitest.StripANSI(out)

	if strings.Contains(plain, "\n") {
		t.Fatalf("expected a single line when totalWidth is 0, got:\n%s", plain)
	}
	if !strings.Contains(plain, value) {
		t.Fatalf("expected the value to render intact, got %q", plain)
	}
}

func TestKeyValueNoteHasNoDots(t *testing.T) {
	out := KeyValueNote("note", "some prose here", 10, 0)
	plain := tuitest.StripANSI(out)
	want := "note" + strings.Repeat(" ", 6) + "some prose here"

	if plain != want {
		t.Fatalf("KeyValueNote = %q, want %q", plain, want)
	}
	if strings.Contains(plain, ".") {
		t.Fatalf("KeyValueNote must not render dot leaders: %q", plain)
	}
}
