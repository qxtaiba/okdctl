package tui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

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

// TestDottedKVKeyNearColumnKeepsValueColumn pins bug 23: a key close enough
// to the column that fewer than three dots remain keeps the shared value
// column with shorter leaders instead of drifting its value one column
// right (post_deploy_60's DNS rows).
func TestDottedKVKeyNearColumnKeepsValueColumn(t *testing.T) {
	const keyColWidth = 30
	short := tuitest.StripANSI(DottedKeyValueFull("api.mycluster.k8s.local", "192.168.1.50", keyColWidth, 0))
	near := tuitest.StripANSI(DottedKeyValueFull("*.apps.mycluster.k8s.local", "192.168.1.20", keyColWidth, 0))

	if si, ni := strings.Index(short, "192."), strings.Index(near, "192."); si != ni {
		t.Fatalf("value columns drift: %d vs %d\n%q\n%q", si, ni, short, near)
	}
}

// TestDottedKVReservesGutterColumn pins bug 22: a value wide enough to fill
// the row wraps one column early, so it never renders flush against the
// enclosing box border — the same gutter errorbox reserves.
func TestDottedKVReservesGutterColumn(t *testing.T) {
	const totalWidth = 60
	value := strings.Repeat("x", 120)
	out := DottedKeyValueFull("console", value, 20, totalWidth)

	for i, line := range strings.Split(tuitest.StripANSI(out), "\n") {
		trimmed := strings.TrimRight(line, " ")
		if got := lipgloss.Width(trimmed); got >= totalWidth {
			t.Fatalf("line %d fills to column %d, want the last column reserved: %q", i, got, line)
		}
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

// Node-op tables' sub-rows (Builder.SubKV) depend on DottedKeyValueSubFull
// muting its key, which was otherwise unverified.
func TestDottedKeyValueSubFullKeyIsMuted(t *testing.T) {
	forced := colorprofile.TrueColor
	outputProfile.Store(&forced)
	t.Cleanup(func() { SetColorProfileFor(&bytes.Buffer{}) })

	full := DottedKeyValueFull("key", "value", DefaultKeyColWidth, 0)
	sub := DottedKeyValueSubFull("key", "value", DefaultKeyColWidth, 0)

	mutedKey := lipgloss.NewStyle().Foreground(ColorTextFaint()).Render("key")
	plainKey := lipgloss.NewStyle().Foreground(ColorTextDim()).Render("key")

	if !strings.Contains(sub, mutedKey) {
		t.Errorf("SubFull's key must render with the muted ColorTextFaint() foreground:\n%q", sub)
	}
	if !strings.Contains(full, plainKey) {
		t.Errorf("Full's key must render with the normal ColorTextDim() foreground:\n%q", full)
	}
	if strings.Contains(sub, plainKey) {
		t.Errorf("SubFull's key must not reuse Full's ColorTextDim() styling:\n%q", sub)
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

	long := tuitest.StripANSI(KeyValueNote("a-very-long-key", "v", 10, 0))
	if long != "a-very-long-key v" {
		t.Fatalf("KeyValueNote overlong key = %q, want exactly one space of separation", long)
	}
}
