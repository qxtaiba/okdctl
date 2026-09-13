package wizard

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestLabelWidthForLongestPlusTwo(t *testing.T) {
	if got := LabelWidthFor("cpu", "control plane data disk"); got != 25 {
		t.Errorf("LabelWidthFor = %d, want 25", got)
	}
	if got := LabelWidthFor("cpu"); got != minLabelWidth {
		t.Errorf("LabelWidthFor = %d, want minLabelWidth %d", got, minLabelWidth)
	}
}

func TestSectionStylesForRendersLongLabelOnOneLine(t *testing.T) {
	st := NewSectionStylesFor(60, "control plane data disk")
	out := tuitest.StripANSI(st.KVPair("control plane data disk", "x"))
	if strings.Contains(out, "\n") {
		t.Errorf("KVPair wrapped a long label: %q", out)
	}
	if idx := strings.Index(out, "x"); idx != 25 {
		t.Errorf("value starts at column %d, want 25: %q", idx, out)
	}
}

func TestSeparatorsSpanFullWidth(t *testing.T) {
	st := NewSectionStyles(60)
	if w := lipgloss.Width(st.Separator); w != 60 {
		t.Errorf("Separator width = %d, want 60", w)
	}
	if w := lipgloss.Width(st.ThickSeparator); w != 60 {
		t.Errorf("ThickSeparator width = %d, want 60", w)
	}
}

func TestRenderSectionSkipsAndRenders(t *testing.T) {
	st := NewSectionStyles(60)
	out := RenderSection(&st, "compute", []KVEntry{
		{Label: "cpu", Value: "4"},
		{Label: "hidden", Value: "x", Skip: true},
	})
	if !strings.Contains(out, "compute") || !strings.Contains(out, "cpu") {
		t.Errorf("section content missing: %q", out)
	}
	if strings.Contains(out, "hidden") {
		t.Errorf("skipped entry rendered: %q", out)
	}
	if RenderSection(&st, "empty", []KVEntry{{Label: "a", Value: "b", Skip: true}}) != "" {
		t.Error("all-skipped section must render empty")
	}
}
