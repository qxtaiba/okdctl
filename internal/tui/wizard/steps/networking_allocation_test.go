package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func previewValues(start string) map[string]string {
	return map[string]string{
		config.FieldNetworkingMachineCIDR: "192.168.1.0/24",
		config.FieldNetworkingNodeIPStart: start,
		config.FieldNetworkingAPIVIP:      "192.168.1.158",
		config.FieldNetworkingIngressVIP:  "192.168.1.159",
	}
}

func TestNetworkAllocationPreviewDerivesTheNodeRanges(t *testing.T) {
	for name, tc := range map[string]struct {
		workers []int
		want    string
	}{
		"two workers":      {[]int{0, 1}, "workers 192.168.1.160–192.168.1.161"},
		"one worker":       {[]int{0}, "workers 192.168.1.160\n"},
		"hole in the pool": {[]int{0, 3}, "workers 192.168.1.160, 192.168.1.163"},
	} {
		t.Run(name, func(t *testing.T) {
			got := tuitest.StripANSI(renderNetworkAllocationPreview(previewValues("192.168.1.150"), tc.workers))
			for _, want := range []string{"masters 192.168.1.150–192.168.1.152", tc.want, "api vip 192.168.1.158", "ingress vip 192.168.1.159"} {
				if !strings.Contains(got, want) {
					t.Errorf("preview = %q, want %q", got, want)
				}
			}
			if strings.Contains(got, "leaves machine cidr") {
				t.Errorf("preview flags an in-range layout: %q", got)
			}
		})
	}

	none := tuitest.StripANSI(renderNetworkAllocationPreview(previewValues("192.168.1.150"), nil))
	if strings.Contains(none, "workers") {
		t.Errorf("preview with no workers lists a worker range: %q", none)
	}
}

func TestNetworkAllocationPreviewWarnsOutsideTheCIDRAndOnBadStart(t *testing.T) {
	for name, tc := range map[string]struct {
		start   string
		workers []int
	}{
		"contiguous": {"192.168.1.250", []int{0, 1}},
		"hole":       {"192.168.1.240", []int{0, 6}},
	} {
		leaves := tuitest.StripANSI(renderNetworkAllocationPreview(previewValues(tc.start), tc.workers))
		if !strings.Contains(leaves, "node range leaves machine cidr 192.168.1.0/24") {
			t.Errorf("%s: preview = %q, want the out-of-range warning", name, leaves)
		}
	}
	invalid := tuitest.StripANSI(renderNetworkAllocationPreview(previewValues("not-an-ip"), []int{0}))
	if !strings.Contains(invalid, "invalid node ip start") || strings.Contains(invalid, "masters") {
		t.Errorf("preview = %q, want invalid input with no fabricated allocations", invalid)
	}
}

func TestNetworkingAllocationPreviewTracksTypedStartIP(t *testing.T) {
	cfg := config.DefaultConfig()
	capacity := &WizardCapacitySnapshot{cfg: cfg}
	step := NewNetworkingStep(capacity)
	step.LoadFromConfig(cfg, true)
	step.Init()
	initial := tuitest.StripANSI(step.View(120, 40))
	if !strings.Contains(initial, "masters 192.168.1.150–192.168.1.152") {
		t.Fatalf("initial view has no allocation preview:\n%s", initial)
	}
	for range 5 {
		step.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	step.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	step.Update(tea.PasteMsg{Content: "40"})
	if got := step.Value(config.FieldNetworkingNodeIPStart); got != "192.168.1.140" {
		t.Fatalf("node ip start after key input = %q, want 192.168.1.140", got)
	}
	updated := tuitest.StripANSI(step.View(120, 40))
	for _, want := range []string{"masters 192.168.1.140–192.168.1.142", "workers 192.168.1.150–192.168.1.151"} {
		if !strings.Contains(updated, want) {
			t.Errorf("updated view has no %q allocation:\n%s", want, updated)
		}
	}
}

func TestNetworkingWithoutASessionConfigOmitsAllocationPreview(t *testing.T) {
	for name, step := range map[string]interface{ View(int, int) string }{
		"nil capacity":    NewNetworkingStep(nil),
		"capacity no cfg": NewNetworkingStep(&WizardCapacitySnapshot{}),
	} {
		if got := step.View(100, 30); strings.Contains(got, "allocation preview") {
			t.Fatalf("%s: view shows a fabricated preview:\n%s", name, got)
		}
	}
}
