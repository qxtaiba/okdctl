package wizard

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func ribbonItems() []KeyBinding {
	return []KeyBinding{
		{"↑/↓", "navigate"},
		{"tab", "next"},
		{"enter", "continue"},
		{"esc", "back"},
		{"?", "help"},
		{"pgup/pgdn", "scroll"},
	}
}

func TestRenderHelpRibbon_FitsExactly(t *testing.T) {
	out := tuitest.StripANSI(RenderHelpRibbon(ribbonItems(), 40))
	if lipgloss.Width(out) > 40 || strings.Contains(out, "\n") || !strings.HasSuffix(out, "…") {
		t.Fatalf("%q", out)
	}
}

func TestRenderHelpRibbon_NoEllipsisWhenAllFit(t *testing.T) {
	items := ribbonItems()
	out := tuitest.StripANSI(RenderHelpRibbon(items, 120))
	if strings.Contains(out, "…") {
		t.Fatalf("unexpected ellipsis: %q", out)
	}
	for _, it := range items {
		if !strings.Contains(out, it.Key) || !strings.Contains(out, it.Help) {
			t.Fatalf("missing %q/%q in %q", it.Key, it.Help, out)
		}
	}
}
