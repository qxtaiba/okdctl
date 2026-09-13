package cli

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

func TestPrintLeadersIndentAndWrap(t *testing.T) {
	tui.SetTerminalWidth(60)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	long := strings.Repeat("wrapme ", 20)

	cases := []struct {
		name string
		key  string
	}{
		{"short key", "key"},
		{"long key engages dotsNeeded floor", strings.Repeat("k", 24)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := printLeaders(&buf, [][2]string{{tc.key, long}}); err != nil {
				t.Fatalf("printLeaders() error = %v", err)
			}

			lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
			if len(lines) < 2 {
				t.Fatalf("expected the long value to wrap across multiple lines, got %d: %q", len(lines), buf.String())
			}

			wantContIndent := 2 + lipgloss.Width(tui.DottedKeyValueFull(tc.key, "", tui.DefaultKeyColWidth, 0))

			for i, line := range lines {
				if w := lipgloss.Width(line); w > 60 {
					t.Errorf("line %d exceeds 60 cols (width %d): %q", i, w, line)
				}

				wantIndent := 2
				if i > 0 {
					wantIndent = wantContIndent
				}
				gotIndent := len(line) - len(strings.TrimLeft(line, " "))
				if gotIndent != wantIndent {
					t.Errorf("line %d indent = %d spaces, want exactly %d: %q", i, gotIndent, wantIndent, line)
				}
				if strings.TrimLeft(line, " ") == "" {
					t.Errorf("line %d is all spaces: %q", i, line)
				}
			}
		})
	}
}
