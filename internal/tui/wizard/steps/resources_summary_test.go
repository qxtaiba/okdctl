package steps

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestResourceSummaryRebindsOnPolarityFlip(t *testing.T) {
	t.Cleanup(func() { tui.SetDarkBackground(true) })

	step, state := NewResourcesStep(nil)
	state.Cfg = config.DefaultConfig()
	step.LoadFromConfig(state.Cfg, true)
	tui.SetDarkBackground(false)

	out := renderResourceFooter(step, state, 120)
	if !strings.Contains(out, "126;34;206") {
		t.Errorf("resource summary = %q, want the light Primary tier (#7E22CE)", out)
	}
	if strings.Contains(out, "147;51;234") {
		t.Errorf("resource summary = %q, still carries the dark Primary tier (#9333EA)", out)
	}
}

func TestResourcePinnedFooterSumsTheInventoryAgainstOnlineCapacity(t *testing.T) {
	cfg := config.DefaultConfig()
	capacity := &WizardCapacitySnapshot{cfg: cfg, discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "pve1", Status: "online", CPUs: 4, CPUsKnown: true, MemGB: 8, MemKnown: true},
		{Name: "pve2", Status: "online", CPUs: 8, CPUsKnown: true, MemGB: 16, MemKnown: true},
		{Name: "pve3", Status: "offline", CPUs: 64, CPUsKnown: true, MemGB: 256, MemKnown: true},
	}}}
	step, state := NewResourcesStep(capacity)
	state.Cfg = cfg
	step.LoadFromConfig(cfg, true)

	got := tuitest.StripANSI(step.PinnedFooter(120))
	for _, want := range []string{"28 vcpu", "80 gb ram", "1600 gb disk", "exceeds online capacity", "12c/24g"} {
		if !strings.Contains(got, want) {
			t.Errorf("PinnedFooter() = %q, want %q", got, want)
		}
	}
}

func TestGolden_ResourcesFooterBranches(t *testing.T) {
	cases := []struct {
		name      string
		discovery *proxmoxDiscovery
		want      []string
	}{
		{
			name: "healthy",
			discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
				{Name: "pve1", Status: "online", CPUs: 64, CPUsKnown: true, MemGB: 256, MemKnown: true},
			}},
			want: []string{"12 vcpu", "48 gb ram", "360 gb disk"},
		},
		{
			name: "exceeds",
			discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
				{Name: "pve1", Status: "online", CPUs: 4, CPUsKnown: true, MemGB: 8, MemKnown: true},
			}},
			want: []string{"12 vcpu", "exceeds online capacity"},
		},
		{
			name: "unknown",
			discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
				{Name: "pve1", Status: "online", CPUsKnown: false, MemKnown: false},
			}},
			want: []string{"12 vcpu", "online capacity unknown"},
		},
	}

	for _, tc := range cases {
		for _, width := range []int{80, 180} {
			t.Run(fmt.Sprintf("%s_w%d", tc.name, width), func(t *testing.T) {
				cfg := config.MinimalConfig()
				capacity := &WizardCapacitySnapshot{cfg: cfg, discovery: tc.discovery}
				step, state := NewResourcesStep(capacity)
				state.Cfg = cfg
				step.LoadFromConfig(cfg, true)

				footer := step.PinnedFooter(width)
				tuitest.Golden(t, fmt.Sprintf("resources-footer-%s_w%d", tc.name, width), footer)
				tuitest.AssertFits(t, footer, width, 0)

				plain := tuitest.StripANSI(footer)
				for _, want := range tc.want {
					if !strings.Contains(plain, want) {
						t.Errorf("%s footer at width %d is missing %q:\n%s", tc.name, width, want, plain)
					}
				}
			})
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
	if got := tuitest.StripANSI(step.PinnedFooter(100)); !strings.Contains(got, "31 vcpu") {
		t.Fatalf("footer after entering 5 vcpus = %q, want totals recalculated from the message", got)
	}
	step.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	step.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	step.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	for range 3 {
		step.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	step.Update(tea.PasteMsg{Content: "160"})
	if got := tuitest.StripANSI(step.PinnedFooter(120)); !strings.Contains(got, "1720 gb disk") {
		t.Fatalf("footer after changing the control-plane root disk = %q, want 1720 gb disk", got)
	}
	if got := cfg.ControlPlane.CPU; got != 4 {
		t.Fatalf("the footer wrote %d into the session config; totals must come from an overlay", got)
	}
}

func TestSetDiskSize(t *testing.T) {
	disks := []config.DiskSpec{{SizeGiB: 120}, {SizeGiB: 500, Storage: "tank"}}
	if got := setDiskSize(disks, 0, 160); got[0].SizeGiB != 160 || len(got) != 2 {
		t.Errorf("root resize = %+v", got)
	}
	if got := setDiskSize(disks, 1, 0); len(got) != 1 {
		t.Errorf("zero data disk = %+v, want the root alone", got)
	}
	if got := setDiskSize([]config.DiskSpec{{SizeGiB: 120}}, 1, 0); len(got) != 1 {
		t.Errorf("zero on an absent data disk = %+v, want unchanged", got)
	}
	if got := setDiskSize([]config.DiskSpec{{SizeGiB: 120}}, 1, 300); len(got) != 2 || got[1].SizeGiB != 300 {
		t.Errorf("append data disk = %+v", got)
	}
	if got := setDiskSize(nil, 0, 100); len(got) != 1 || got[0].SizeGiB != 100 {
		t.Errorf("root on nil = %+v", got)
	}
}
