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

func TestResourcesAndReviewTotalsAgree(t *testing.T) {
	for _, tc := range []struct {
		name      string
		bootstrap config.NodeConfig
		want      [3]string
	}{
		{"bootstrap sized explicitly", config.NodeConfig{Count: 1, CPU: 8, MemoryMB: 16384, DiskGB: 80}, [3]string{"42", "96", "480"}},
		{"bootstrap omitted inherits control plane", config.NodeConfig{}, [3]string{"40", "96", "480"}},
		{"bootstrap vcpus only inherits memory", config.NodeConfig{CPU: 2}, [3]string{"36", "96", "480"}},
		{"bootstrap memory only inherits vcpus", config.NodeConfig{MemoryMB: 8192}, [3]string{"40", "88", "480"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := resourceParityTestConfig()
			cfg.Topology.Bootstrap = tc.bootstrap

			step, state := NewResourcesStep(nil)
			state.Cfg = cfg
			step.LoadFromConfig(cfg, true)
			resourcesOut := tuitest.StripANSI(step.PinnedFooter(120))

			review := NewReviewStep()
			review.SetConfig(cfg)
			reviewOut := tuitest.StripANSI(review.View(120, 100))

			rCPU, rRAM, rDisk := parseResourceTotals(t, resourcesOut)
			vCPU, vRAM, vDisk := parseResourceTotals(t, reviewOut)

			if got := [3]string{rCPU, rRAM, rDisk}; got != tc.want {
				t.Errorf("resources totals = %v vcpu/gb ram/gb disk, want %v", got, tc.want)
			}
			if got := [3]string{vCPU, vRAM, vDisk}; got != tc.want {
				t.Errorf("review totals = %v vcpu/gb ram/gb disk, want %v", got, tc.want)
			}
		})
	}
}

func TestResourcePinnedFooterShowsNoTotalsWithoutTheSessionConfig(t *testing.T) {
	step, _ := NewResourcesStep(nil)

	if got := tuitest.StripANSI(step.PinnedFooter(120)); got != "" {
		t.Errorf("PinnedFooter() = %q without a config, want no totals rather than compiled-in defaults", got)
	}
}
