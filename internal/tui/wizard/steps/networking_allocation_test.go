package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestNetworkAllocationPreviewUsesCurrentValues(t *testing.T) {
	got := tuitest.StripANSI(renderNetworkAllocationPreview(map[string]string{
		"machine_cidr": "192.168.1.0/24",
		"start_ip":     "192.168.1.140",
		"gateway":      "192.168.1.1",
		"bastion_ip":   "192.168.1.20",
	}, 2, 1))
	for _, want := range []string{"bootstrap 192.168.1.140", "masters 192.168.1.141–192.168.1.142", "worker0 192.168.1.143", "api vip 192.168.1.10 (auto)", "vm dns → bastion 192.168.1.20"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview = %q, want %q", got, want)
		}
	}
}

func TestNetworkingAllocationPreviewTracksTypedStartIP(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.Count = 1
	cfg.Topology.Workers.Count = 1
	capacity := &WizardCapacitySnapshot{cfg: cfg, discovery: demoDiscovery()}
	step := NewNetworkingStep(capacity)
	step.LoadFromConfig(cfg, true)
	step.Init()
	initial := tuitest.StripANSI(step.View(120, 40))
	if !strings.Contains(initial, "bootstrap 192.168.1.140") {
		t.Fatalf("initial view has no actual allocation preview:\n%s", initial)
	}
	for range 6 {
		step.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	step.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	step.Update(tea.PasteMsg{Content: "50"})
	if got := step.Value("start_ip"); got != "192.168.1.150" {
		t.Fatalf("start_ip after key input = %q, want 192.168.1.150", got)
	}
	updated := tuitest.StripANSI(step.View(120, 40))
	for _, want := range []string{"bootstrap 192.168.1.150", "master 192.168.1.151", "worker0 192.168.1.152"} {
		if !strings.Contains(updated, want) {
			t.Errorf("updated view has no %q allocation:\n%s", want, updated)
		}
	}
}

func TestNetworkingWithoutDiscoveryOmitsAllocationPreview(t *testing.T) {
	step := NewNetworkingStep()
	if got := step.View(100, 30); strings.Contains(got, "allocation preview") {
		t.Fatalf("view without discovery shows a fabricated preview:\n%s", got)
	}
}

func TestNetworkAllocationPreviewMarksCollisionsAndInvalidStart(t *testing.T) {
	collision := tuitest.StripANSI(renderNetworkAllocationPreview(map[string]string{
		"machine_cidr": "192.168.1.0/24",
		"start_ip":     "192.168.1.140",
		"gateway":      "192.168.1.141",
		"bastion_ip":   "192.168.1.20",
	}, 1, 0))
	if !strings.Contains(collision, "collision") || !strings.Contains(collision, "192.168.1.141") {
		t.Fatalf("preview = %q, want the gateway/static allocation collision", collision)
	}

	invalid := tuitest.StripANSI(renderNetworkAllocationPreview(map[string]string{
		"machine_cidr": "192.168.1.0/24",
		"start_ip":     "not-an-ip",
		"gateway":      "192.168.1.1",
		"bastion_ip":   "192.168.1.20",
	}, 1, 1))
	if !strings.Contains(invalid, "invalid start IP") || strings.Contains(invalid, "192.168.1.10") || strings.Contains(invalid, "192.168.1.11") {
		t.Fatalf("preview = %q, want invalid input with no fabricated allocations", invalid)
	}
}

func TestNetworkAllocationPreviewMarksOutOfRangeReservedAddresses(t *testing.T) {
	got := tuitest.StripANSI(renderNetworkAllocationPreview(map[string]string{
		"machine_cidr": "192.168.1.0/24",
		"start_ip":     "192.168.1.140",
		"gateway":      "192.168.1.1",
		"bastion_ip":   "10.0.0.20",
		"vip":          "10.0.0.10",
	}, 1, 0))
	for _, want := range []string{"api vip outside machine CIDR: 10.0.0.10", "bastion outside machine CIDR: 10.0.0.20"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview = %q, want %q", got, want)
		}
	}
}
