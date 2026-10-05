package steps

import (
	"regexp"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// resourceTotalsPattern matches the "N vcpu, N gb ram, N gb disk" triple each
// screen's total line renders, tolerating either screen's separator style
// (review uses commas, the resources footer uses middots).
var resourceTotalsPattern = regexp.MustCompile(`(\d+)\s*vcpu\D*?(\d+)\s*gb ram\D*?(\d+)\s*gb disk`)

// parseResourceTotals extracts the vcpu/ram-gb/disk-gb triple from a
// rendered screen, for the cross-screen agreement assertion below.
func parseResourceTotals(t *testing.T, text string) (cpu, ramGB, diskGB string) {
	t.Helper()
	m := resourceTotalsPattern.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no resource totals found in:\n%s", text)
	}
	return m[1], m[2], m[3]
}

// resourceParityTestConfig returns non-default control-plane, worker, and
// bootstrap sizing chosen so a hardcoded bootstrap constant — the kind
// review.go's old computeTotals used — silently diverges from the real
// bootstrap allocation derived from cfg.Topology.Bootstrap.
func resourceParityTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane = config.NodeConfig{Count: 3, CPU: 6, MemoryMB: 16384, DiskGB: 80}
	cfg.Topology.Workers = config.NodeConfig{Count: 2, CPU: 8, MemoryMB: 16384, DiskGB: 80}
	cfg.Topology.Bootstrap = config.NodeConfig{Count: 1, CPU: 8, MemoryMB: 16384, DiskGB: 80}
	cfg.Disks.WorkerDataSizeGB = 0
	cfg.Disks.ControlPlaneDataSizeGB = 0
	return cfg
}

// TestResourcesAndReviewTotalsAgree is the permanent cross-screen agreement
// test: with identical, non-default control-plane/bootstrap sizing, the
// resources screen and the review screen must render the exact same
// vcpu/ram/disk totals. It pins the bug where review.go computed its own
// totals with a hardcoded bootstrap constant instead of deriving them from
// cfg.Topology.Bootstrap like the resources screen does.
func TestResourcesAndReviewTotalsAgree(t *testing.T) {
	cfg := resourceParityTestConfig()

	step, state := NewResourcesStep(nil)
	state.Cfg = cfg
	step.LoadFromConfig(cfg, true)
	resourcesOut := tuitest.StripANSI(step.PinnedFooter(120))

	review := NewReviewStep()
	review.SetConfig(cfg)
	reviewOut := tuitest.StripANSI(review.View(120, 100))

	rCPU, rRAM, rDisk := parseResourceTotals(t, resourcesOut)
	vCPU, vRAM, vDisk := parseResourceTotals(t, reviewOut)

	if rCPU != vCPU || rRAM != vRAM || rDisk != vDisk {
		t.Fatalf("resources and review totals disagree: resources=%s vcpu/%s gb ram/%s gb disk, review=%s vcpu/%s gb ram/%s gb disk",
			rCPU, rRAM, rDisk, vCPU, vRAM, vDisk)
	}
}
