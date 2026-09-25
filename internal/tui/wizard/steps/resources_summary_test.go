package steps

import (
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// TestResourceSummaryRebindsOnPolarityFlip pins the totals line against the
// dark-frozen-capture bug class: after a light flip its brand-highlighted
// values must carry the light Primary tier, not an init-captured dark one.
func TestResourceSummaryRebindsOnPolarityFlip(t *testing.T) {
	t.Cleanup(func() { tui.SetDarkBackground(true) })

	step, state := NewResourcesStep()
	tui.SetDarkBackground(false)

	out := renderResourceSummary(step, state)
	if !strings.Contains(out, "126;34;206") {
		t.Errorf("resource summary = %q, want the light Primary tier (#7E22CE)", out)
	}
	if strings.Contains(out, "147;51;234") {
		t.Errorf("resource summary = %q, still carries the dark Primary tier (#9333EA)", out)
	}
}
