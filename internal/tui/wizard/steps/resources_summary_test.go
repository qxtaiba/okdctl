package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

// TestResourceSummaryRebindsOnPolarityFlip pins the totals line against the
// dark-frozen-capture bug class: after a light flip its brand-highlighted
// values must carry the light Primary tier, not an init-captured dark one.
func TestResourceSummaryRebindsOnPolarityFlip(t *testing.T) {
	t.Cleanup(func() { tui.SetDarkBackground(true) })

	step, state := NewResourcesStep(nil)
	tui.SetDarkBackground(false)

	out := renderResourceFooter(step, state, 120)
	if !strings.Contains(out, "126;34;206") {
		t.Errorf("resource summary = %q, want the light Primary tier (#7E22CE)", out)
	}
	if strings.Contains(out, "147;51;234") {
		t.Errorf("resource summary = %q, still carries the dark Primary tier (#9333EA)", out)
	}
}

func TestResourcePinnedFooterIncludesBootstrapAndOnlineCapacity(t *testing.T) {
	cfg := config.MinimalConfig()
	cfg.Topology.ControlPlane.Count = 2
	cfg.Topology.Workers.Count = 1
	cfg.Topology.Workers.CPU = 8
	cfg.Topology.Workers.MemoryMB = 8192
	cfg.Topology.Workers.DiskGB = 50
	cfg.Disks.WorkerDataSizeGB = 0
	cfg.Topology.Bootstrap = config.NodeConfig{Count: 1, CPU: 4, MemoryMB: 8192, DiskGB: 50}
	capacity := &WizardCapacitySnapshot{cfg: cfg, discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "pve1", Status: "online", CPUs: 4, CPUsKnown: true, MemGB: 8, MemKnown: true},
		{Name: "pve2", Status: "online", CPUs: 8, CPUsKnown: true, MemGB: 16, MemKnown: true},
		{Name: "pve3", Status: "offline", CPUs: 64, CPUsKnown: true, MemGB: 256, MemKnown: true},
	}}}
	step, state := NewResourcesStep(capacity)
	state.Cfg = cfg
	step.LoadFromConfig(cfg, true)

	got := tuitest.StripANSI(step.PinnedFooter(120))
	for _, want := range []string{"20 vcpu", "32 gb ram", "200 gb disk", "exceeds online capacity", "12c/24g"} {
		if !strings.Contains(got, want) {
			t.Errorf("PinnedFooter() = %q, want %q", got, want)
		}
	}
}

func TestResourcePinnedFooterTracksKeyInput(t *testing.T) {
	cfg := config.DefaultConfig()
	step, state := NewResourcesStep(nil)
	state.Cfg = cfg
	step.LoadFromConfig(cfg, true)
	step.Init()
	step.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := tuitest.StripANSI(step.PinnedFooter(100)); !strings.Contains(got, "pending") {
		t.Fatalf("footer after clearing a required resource = %q, want pending", got)
	}
	step.Update(tea.PasteMsg{Content: "5"})
	if got := tuitest.StripANSI(step.PinnedFooter(100)); !strings.Contains(got, "43 vcpu") {
		t.Fatalf("footer after entering 5 vcpus = %q, want totals recalculated from the message", got)
	}
	step.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	step.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	step.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	step.Update(tea.PasteMsg{Content: "60"})
	if got := tuitest.StripANSI(step.PinnedFooter(120)); !strings.Contains(got, "1890 gb disk") {
		t.Fatalf("footer after changing control-plane disk = %q, want bootstrap disk to follow it", got)
	}
}
