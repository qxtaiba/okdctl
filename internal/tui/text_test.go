package tui

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

func TestTruncateMiddleIsRuneSafe(t *testing.T) {
	t.Run("cjk and combining marks stay valid", func(t *testing.T) {
		combining := strings.Repeat("é", 5)
		cjk := strings.Repeat("中", 15)
		input := combining + cjk + "tail"

		got := truncateMiddle(input, 20)

		if !utf8.ValidString(got) {
			t.Fatalf("truncateMiddle produced invalid UTF-8: %q", got)
		}
		if w := lipgloss.Width(got); w > 20 {
			t.Fatalf("truncateMiddle width %d exceeds cap 20: %q", w, got)
		}
		if !strings.Contains(got, "…") {
			t.Fatalf("truncateMiddle should contain an ellipsis: %q", got)
		}
		if !strings.HasSuffix(got, "tail") {
			t.Fatalf("truncateMiddle should keep the distinguishing tail: %q", got)
		}
	})

	// The backward scan must not cut between a combining mark's base rune
	// and the mark itself: a mark stranded at the front of the tail would
	// visually attach to the preceding ellipsis instead of its own base.
	t.Run("backward scan through a decomposed sequence stays mark-safe", func(t *testing.T) {
		cjk := strings.Repeat("中", 15)
		input := cjk + "éí"

		got := truncateMiddle(input, 3)

		if !utf8.ValidString(got) {
			t.Fatalf("truncateMiddle produced invalid UTF-8: %q", got)
		}
		if w := lipgloss.Width(got); w > 3 {
			t.Fatalf("truncateMiddle width %d exceeds cap 3: %q", w, got)
		}
		idx := strings.Index(got, "…")
		if idx < 0 {
			t.Fatalf("truncateMiddle should contain an ellipsis: %q", got)
		}
		if after := []rune(got[idx+len("…"):]); len(after) > 0 && unicode.IsMark(after[0]) {
			t.Fatalf("tail must not begin with a stranded combining mark: %q", got)
		}
	})
}

func TestTruncateMiddleDropsAWideCellStraddlingTheTailCut(t *testing.T) {
	input := "abcdefghij" + strings.Repeat("中", 15) + "tail"

	if got, want := truncateMiddle(input, 19), "abcdefghi…中中tail"; got != want {
		t.Errorf("truncateMiddle = %q, want %q", got, want)
	}
}

func TestTruncateMiddleFoldsInvalidUTF8(t *testing.T) {
	if got, want := truncateMiddle("abc\xffdef\xfeghi", 8), "abc…�ghi"; got != want {
		t.Errorf("truncateMiddle = %q, want %q", got, want)
	}
}

func TestPromptLine(t *testing.T) {
	got := PromptLine("proceed with destroy? [y/N]")
	if !strings.Contains(got, IconPointer) {
		t.Errorf("PromptLine must carry the pointer glyph: %q", got)
	}
	if !strings.HasSuffix(got, "proceed with destroy? [y/N]: ") {
		t.Errorf("PromptLine must append a trailing colon and space to the text: %q", got)
	}
}

// TestWrapLinesNeverBreaksAtHyphens pins that copy-paste recovery commands
// (`okdctl deploy --fresh`, kubeadmin-password paths) wrap on spaces only:
// a hyphen split silently corrupts the exact strings users copy during
// failure recovery.
func TestWrapLinesNeverBreaksAtHyphens(t *testing.T) {
	cases := []struct {
		text  string
		width int
		token string
	}{
		{"resume with okdctl deploy --fresh after cleanup", 16, "--fresh"},
		{"cat cluster-config/auth/kubeadmin-password to log in", 40, "cluster-config/auth/kubeadmin-password"},
		{"resume the interrupted power-cycle with okdctl node resize", 28, "power-cycle"},
	}
	for _, tc := range cases {
		lines := WrapLines(tc.text, tc.width)
		found := false
		for _, l := range lines {
			if w := lipgloss.Width(l); w > tc.width {
				t.Fatalf("width %d: %d-col line %q", tc.width, w, l)
			}
			if strings.Contains(l, tc.token) {
				found = true
			}
		}
		if !found {
			t.Fatalf("width %d: token %q split across lines: %q", tc.width, tc.token, lines)
		}
	}
}

func TestWrapLinesHardSplitsLongToken(t *testing.T) {
	long := strings.Repeat("中", 30)

	lines := WrapLines(long, 20)

	for _, l := range lines {
		if w := lipgloss.Width(l); w > 20 {
			t.Fatalf("WrapLines produced a %d-col line over the 20 budget: %q", w, l)
		}
	}
	if joined := strings.Join(lines, ""); joined != long {
		t.Fatalf("WrapLines lost characters: %q", joined)
	}
}
