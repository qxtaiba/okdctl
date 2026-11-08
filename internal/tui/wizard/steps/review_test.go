package steps

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func reviewTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Nodes = []string{"pve1", "pve2"}
	return cfg
}

func TestReviewStep_DerivesEverySectionFromTheModel(t *testing.T) {
	s := NewReviewStep()
	s.SetConfig(reviewTestConfig())

	out := tuitest.StripANSI(s.View(120, 100))
	for _, want := range []string{
		"cluster identity", "proxmox", "nodes", "networking", "compute", "files", "advanced", "WRITES",
		"lab-master0", "6010", "192.168.1.150", "pve1",
		"lab-worker1", "6101", "192.168.1.161", "pve2",
		"3 × (4 vcpu, 16 gb ram, disks 120 gb)",
		"2 × (8 vcpu, 16 gb ram, disks 120+500 gb)",
		"28 vcpu, 80 gb ram, 1600 gb disk",
		"drain timeout", "10m", "red hat catalogs", "disabled",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q:\n%s", want, out)
		}
	}
}

func TestReviewStep_ShowsChangesFromSavedConfigByJSONPath(t *testing.T) {
	saved := reviewTestConfig()
	cfg := reviewTestConfig()
	s := NewReviewStep()
	s.SetConfig(cfg)
	s.SetSavedConfig(saved)

	cfg.Cluster.Domain = "prod.example"
	cfg.ControlPlane.CPU = 8
	cfg.Nodes = map[string]config.NodeOverride{"lab-master0": {Host: "pve2"}}

	frame := s.View(120, 100)
	tuitest.AssertFits(t, frame, 120, 100)
	out := tuitest.StripANSI(frame)
	for _, want := range []string{
		"CONFIG CHANGES · 3",
		"cluster.domain", "k8s.local → prod.example",
		"control_plane.cpu", "4 → 8",
		"nodes.lab-master0.host", " → pve2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing config change %q:\n%s", want, out)
		}
	}
}

func TestReviewStep_NoChangesWhenSavedMatches(t *testing.T) {
	cfg := reviewTestConfig()
	s := NewReviewStep()
	s.SetConfig(cfg)
	s.SetSavedConfig(cfg)
	if out := tuitest.StripANSI(s.View(120, 100)); strings.Contains(out, "CONFIG CHANGES") {
		t.Fatalf("identical configs rendered a diff:\n%s", out)
	}
}

func TestReviewConfigSnapshotFlattensJSONPaths(t *testing.T) {
	snap := reviewConfigSnapshot(reviewTestConfig())
	for key, want := range map[string]string{
		"cluster.name":              "lab",
		"provider.proxmox.nodes":    "pve1, pve2",
		"workers.disks[1].size_gib": "500",
		"workers.disks[1].wipe":     "true",
		"lifecycle.drain_timeout":   "10m0s",
		"handoff.enabled":           "true",
		"networking.ntp":            "",
	} {
		if got := snap[key]; got != want {
			t.Errorf("snapshot[%q] = %q, want %q", key, got, want)
		}
	}
	if reviewConfigSnapshot(nil) != nil {
		t.Error("nil config must snapshot to nil")
	}
}

func TestReviewStep_SanitizesHostileProxmoxText(t *testing.T) {
	const payload = "\x1b[2J\x1b[H"
	saved := reviewTestConfig()
	cfg := reviewTestConfig()
	cfg.Provider.Proxmox.Nodes = []string{"pve1" + payload, "pve2"}
	cfg.Nodes = map[string]config.NodeOverride{"lab-master1": {Host: "pve1" + payload}}
	s := NewReviewStep()
	s.SetConfig(cfg)
	s.SetSavedConfig(saved)

	frame := s.View(100, 100)
	if strings.Contains(frame, payload) {
		t.Fatalf("View() carries the raw clear-screen/cursor-home payload:\n%q", frame)
	}
	if !strings.Contains(frame, "�") {
		t.Fatalf("View() shows no sanitization marker for the tampered node:\n%q", frame)
	}
}

func TestReviewStep_PreflightChecksSelectedHostCapacity(t *testing.T) {
	cfg := reviewTestConfig()
	cfg.Nodes = map[string]config.NodeOverride{
		"lab-master0": {Host: "pve1"}, "lab-master1": {Host: "pve1"}, "lab-master2": {Host: "pve1"},
		"lab-worker0": {Host: "pve2"}, "lab-worker1": {Host: "missing"},
	}
	snapshot := &WizardCapacitySnapshot{discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "pve1", Status: "online", CPUs: 8, CPUsKnown: true, MemGB: 32, MemKnown: true},
		{Name: "pve2", Status: "online", CPUs: 24, CPUsKnown: true, MemGB: 96, MemKnown: true},
	}}}
	s := NewReviewStep()
	s.SetConfig(cfg)
	s.SetCapacity(snapshot)

	frame := s.View(70, 100)
	tuitest.AssertFits(t, frame, 70, 100)
	out := tuitest.StripANSI(frame)
	for _, want := range []string{"selected capacity", "pve1 over capacity", "pve2 fits", "missing unavailable"} {
		if !strings.Contains(out, want) {
			t.Errorf("review omitted capacity status %q:\n%s", want, out)
		}
	}
}

func TestReviewStep_NodesUnavailableWhenTheModelCannotBuild(t *testing.T) {
	cfg := reviewTestConfig()
	cfg.Networking.MachineCIDR = "not-a-cidr"
	s := NewReviewStep()
	s.SetConfig(cfg)

	out := tuitest.StripANSI(s.View(100, 100))
	if !strings.Contains(out, "node identities unavailable") || !strings.Contains(out, "configuration · ") {
		t.Fatalf("View() must say why the node table is missing and fail the config check:\n%s", out)
	}
}

func TestReviewStep_HiddenStepGetsNoIndex(t *testing.T) {
	s := NewReviewStep()
	s.SetConfig(reviewTestConfig())
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDBasics, Digit: 1},
		{StepID: wizard.StepIDProxmox, Digit: 2},
	})

	out := s.View(100, 100)
	if !strings.Contains(out, "[2] proxmox") {
		t.Error("View() dropped the jump index for a mapped step")
	}
	if strings.Contains(out, "] files") {
		t.Error("View() shows a jump index for files, want none (hidden step)")
	}
	if !strings.Contains(out, "files") {
		t.Error("View() dropped the files header entirely, want header without an index")
	}
}

func TestReviewStep_DigitKeyEmitsJumpToStepMsg(t *testing.T) {
	s := NewReviewStep()
	s.SetJumpTargets([]wizard.JumpTarget{{StepID: wizard.StepIDProxmox, Digit: 2}})

	_, cmd := s.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if cmd == nil {
		t.Fatal("Update(digit 2) returned a nil cmd, want a JumpToStepMsg command")
	}
	jump, ok := cmd().(wizard.JumpToStepMsg)
	if !ok || jump.StepID != wizard.StepIDProxmox {
		t.Fatalf("Update(digit 2) produced %#v, want JumpToStepMsg{proxmox}", cmd())
	}

	if _, cmd := s.Update(tea.KeyPressMsg{Code: '9', Text: "9"}); cmd != nil {
		if _, ok := cmd().(wizard.JumpToStepMsg); ok {
			t.Fatal("Update(unmapped digit) produced a JumpToStepMsg, want none")
		}
	}
}

func TestReviewStep_ShortHelp_AdvertisesJumpOnlyWhenTargetsExist(t *testing.T) {
	s := NewReviewStep()
	for _, b := range s.ShortHelp() {
		if b.Help == wizard.HelpJump {
			t.Fatal("ShortHelp() advertises jump before any targets are set")
		}
	}
	s.SetJumpTargets([]wizard.JumpTarget{{StepID: wizard.StepIDProxmox, Digit: 1}})
	found := false
	for _, b := range s.ShortHelp() {
		found = found || b.Help == wizard.HelpJump
	}
	if !found {
		t.Error("ShortHelp() does not advertise jump once targets are set")
	}
}

func TestReviewStep_ActionsLiveInTheFooterOnly(t *testing.T) {
	s := NewReviewStep()
	if got := s.PinnedFooter(80); got != "" {
		t.Errorf("PinnedFooter() before SetConfig = %q, want empty", got)
	}
	s.SetConfig(reviewTestConfig())
	footer := s.PinnedFooter(80)
	if !strings.Contains(footer, "deploy now") || !strings.Contains(footer, "save and exit") {
		t.Errorf("PinnedFooter() = %q, want both actions", footer)
	}
	if out := s.View(100, 100); strings.Contains(out, "deploy now") || strings.Contains(out, "save and exit") {
		t.Error("View() body still renders the action radio, want it pinned to the footer only")
	}
	if got := s.GetSelectedAction(); got != wizard.ActionDeploy {
		t.Errorf("GetSelectedAction() = %q, want deploy by default", got)
	}
}

func TestReviewActionsVisibleAfterSummaryWraps(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Endpoint = "https://long-hostname.cluster.example.com:8006"
	step := NewReviewStep()
	step.SetConfig(cfg)
	m := wizard.NewModel([]wizard.WizardStep{step}, cfg)
	m.Init()
	for _, size := range [][2]int{{120, 40}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		out := m.View().Content
		if !strings.Contains(out, "deploy now") || !strings.Contains(out, "save and exit") {
			t.Fatalf("review actions hidden at %v: %s", size, out)
		}
	}
}
