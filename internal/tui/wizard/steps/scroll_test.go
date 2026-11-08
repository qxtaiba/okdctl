package steps

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// bodyRows returns the frame rows between the header and footer rules.
func bodyRows(t *testing.T, frame string) []string {
	t.Helper()
	lines := strings.Split(tuitest.StripANSI(frame), "\n")
	first, last := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "│─") {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 || last <= first {
		t.Fatalf("frame has no header/footer rules:\n%s", strings.Join(lines, "\n"))
	}
	rows := make([]string, 0, last-first-1)
	for _, l := range lines[first+1 : last] {
		rows = append(rows, strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│")))
	}
	return rows
}
