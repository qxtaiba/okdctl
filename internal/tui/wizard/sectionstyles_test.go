package wizard

import (
	"strings"
	"testing"
)

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
