package steps

import (
	"regexp"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

var resourceTotalsPattern = regexp.MustCompile(`(\d+)\s*vcpu\D*?(\d+)\s*gb ram\D*?(\d+)\s*gb disk`)

func parseResourceTotals(t *testing.T, text string) [3]string {
	t.Helper()
	m := resourceTotalsPattern.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no resource totals found in:\n%s", text)
	}
	return [3]string{m[1], m[2], m[3]}
}

func TestResourcesAndReviewTotalsAgreeAndIncludeOverrides(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Nodes = map[string]config.NodeOverride{
		"lab-master0": {MemoryMiB: new(32768)},
		"lab-worker1": {CPU: new(16)},
	}

	step, state := NewResourcesStep(nil)
	state.Cfg = cfg
	step.LoadFromConfig(cfg, true)
	review := NewReviewStep()
	review.SetConfig(cfg)

	want := [3]string{"36", "96", "1600"}
	if got := parseResourceTotals(t, tuitest.StripANSI(step.PinnedFooter(120))); got != want {
		t.Errorf("resources totals = %v, want %v", got, want)
	}
	if got := parseResourceTotals(t, tuitest.StripANSI(review.View(120, 100))); got != want {
		t.Errorf("review totals = %v, want %v", got, want)
	}
}

func TestResourcePinnedFooterShowsNoTotalsWithoutTheSessionConfig(t *testing.T) {
	step, _ := NewResourcesStep(nil)
	if got := tuitest.StripANSI(step.PinnedFooter(120)); got != "" {
		t.Errorf("PinnedFooter() = %q without a config, want no totals", got)
	}
}
