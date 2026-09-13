// Package tuitest renders bubbletea models headlessly and compares frames
// against ANSI-stripped golden files (run OKDCTL_UPDATE_GOLDEN=1 go test
// ./internal/tui/... to accept new frames).
package tuitest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)

// StripANSI removes SGR (color/style) and OSC (e.g. hyperlink) escape
// sequences from s, leaving the plain visible text.
func StripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

// Golden compares the ANSI-stripped got against the golden file for name,
// failing the test on a mismatch; set OKDCTL_UPDATE_GOLDEN to rewrite it
// instead.
func Golden(t *testing.T, name, got string) {
	t.Helper()
	got = StripANSI(got)

	top := strings.SplitN(t.Name(), "/", 2)[0]
	path := filepath.Join("testdata", top, name+".golden")

	if os.Getenv("OKDCTL_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (accept with: OKDCTL_UPDATE_GOLDEN=1 go test ./internal/tui/...)", path, err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden (accept with OKDCTL_UPDATE_GOLDEN=1):\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

func fits(frame string, w, h int) error {
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	if len(lines) > h {
		return fmt.Errorf("frame is %d rows, want <= %d", len(lines), h)
	}
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			return fmt.Errorf("row %d is %d cols, want <= %d: %q", i, lw, w, StripANSI(l))
		}
	}
	return nil
}

// AssertFits fails the test if frame has more than h rows or any row wider
// than w columns (after ANSI stripping).
func AssertFits(t *testing.T, frame string, w, h int) {
	t.Helper()
	if err := fits(frame, w, h); err != nil {
		t.Error(err)
	}
}

// RenderAt resizes m to w×h and returns the resulting frame's content.
func RenderAt(t *testing.T, m tea.Model, w, h int) string {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.View().Content
}
