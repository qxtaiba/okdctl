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
	cfg := &config.Config{}
	cfg.Provider.Proxmox = &config.ProxmoxConfig{
		Host:              "pve.local",
		ControlPlaneNodes: []string{"pve1", "pve2", "pve3"},
		WorkerNodes:       []string{"pve1", "pve2"},
	}
	cfg.Addons = map[string]config.AddonConfig{"flux": {Enabled: true}}
	cfg.Deployment.AutoApprove = true
	return cfg
}

// TestReviewStep_AdvancedShowsAllAppliedSettings pins bug 8: cpu type, numa,
// ha anti-affinity, ntp server, and bin dir are applied by the wizard and
// must be visible at the deploy gate.
func TestReviewStep_AdvancedShowsAllAppliedSettings(t *testing.T) {
	cfg := reviewTestConfig()
	cfg.Provider.Proxmox.CPUType = "x86-64-v2"
	cfg.Provider.Proxmox.NUMAEnabled = true
	cfg.Provider.Proxmox.HAEnabled = true
	cfg.Networking.NTPServer = "pool.ntp.org"
	cfg.Deployment.BinDir = "/opt/okd/bin"
	s := NewReviewStep()
	s.SetConfig(cfg)

	out := tuitest.StripANSI(s.View(100, 100))

	for _, want := range []string{"x86-64-v2", "numa", "ha anti-affinity", "pool.ntp.org", "/opt/okd/bin"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing applied advanced setting %q", want)
		}
	}
}

// TestReviewStep_AddonRowsSortedWithoutSelfDuplication pins bug 9: addon
// rows render in deterministic sorted order, and an addon without detail
// settings never renders its own name as its value.
func TestReviewStep_AddonRowsSortedWithoutSelfDuplication(t *testing.T) {
	cfg := reviewTestConfig()
	cfg.Addons = map[string]config.AddonConfig{
		"secretstore": {Enabled: true, Settings: map[string]string{"type": "vault"}},
		"flux":        {Enabled: true},
	}
	s := NewReviewStep()
	s.SetConfig(cfg)

	out := tuitest.StripANSI(s.View(100, 100))

	fluxAt := strings.Index(out, "flux")
	storeAt := strings.Index(out, "secretstore")
	if fluxAt < 0 || storeAt < 0 {
		t.Fatalf("View() missing addon rows:\n%s", out)
	}
	if fluxAt > storeAt {
		t.Fatalf("addon rows not sorted: flux at %d after secretstore at %d", fluxAt, storeAt)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Count(line, "flux") > 1 {
			t.Fatalf("flux row duplicates its own name: %q", line)
		}
	}
}

func TestReviewStep_ShowsChangesFromSavedConfig(t *testing.T) {
	saved := reviewTestConfig()
	saved.Cluster.Domain = "k8s.local"
	saved.Topology.ControlPlane.CPU = 4
	cfg := reviewTestConfig()
	cfg.Cluster.Domain = saved.Cluster.Domain
	cfg.Topology.ControlPlane.CPU = saved.Topology.ControlPlane.CPU
	cfg.Networking.NTPServer = saved.Networking.NTPServer
	cfg.Deployment.BinDir = saved.Deployment.BinDir
	s := NewReviewStep()
	s.SetConfig(cfg)
	s.SetSavedConfig(saved)

	cfg.Cluster.Domain = "prod.example"
	cfg.Topology.ControlPlane.CPU = 8
	cfg.Networking.NTPServer = "time.example"
	cfg.Deployment.BinDir = "/opt/okd/bin"

	frame := s.View(120, 100)
	tuitest.AssertFits(t, frame, 120, 100)
	out := tuitest.StripANSI(frame)
	for _, want := range []string{
		"CONFIG CHANGES · 4",
		"domain",
		"k8s.local → prod.example",
		"control plane vcpus",
		"4 → 8",
		"ntp server",
		"→ time.example",
		"binary directory",
		"→ /opt/okd/bin",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing config change %q:\n%s", want, out)
		}
	}
}

func TestReviewStep_ShowsNewProviderValues(t *testing.T) {
	saved := reviewTestConfig()
	saved.Provider.Proxmox = nil
	cfg := reviewTestConfig()
	cfg.Provider.Proxmox.AdditionalNetworks = []config.AdditionalNetwork{{Bridge: "vmbr1", Model: "virtio", VLANTag: 42}}
	cfg.Provider.Proxmox.Password.Set("new-secret")
	t.Cleanup(cfg.Provider.Proxmox.Password.Zeroize)
	cfg.Provider.Proxmox.APIToken.Set("api-secret")
	t.Cleanup(cfg.Provider.Proxmox.APIToken.Zeroize)
	cfg.Addons["secretstore"] = config.AddonConfig{Enabled: true, Settings: map[string]string{"token": "addon-secret"}}
	s := NewReviewStep()
	s.SetConfig(cfg)
	s.SetSavedConfig(saved)

	frame := s.View(80, 100)
	tuitest.AssertFits(t, frame, 80, 100)
	out := tuitest.StripANSI(frame)
	for _, want := range []string{"proxmox host", "pve.local", "additional_networks.1", "vmbr1 / virtio / vlan 42"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() omitted newly configured value %q:\n%s", want, out)
		}
	}
	for _, secret := range []string{"new-secret", "api-secret", "addon-secret"} {
		if strings.Contains(out, secret) {
			t.Errorf("View() exposed a credential %q:\n%s", secret, out)
		}
	}
}

func TestReviewStep_PreflightChecksSelectedNodeCapacity(t *testing.T) {
	cfg := reviewTestConfig()
	cfg.Provider.Proxmox.Node = "pve1"
	cfg.Provider.Proxmox.ControlPlaneNodes = []string{"pve1", "pve1", "pve1"}
	cfg.Provider.Proxmox.WorkerNodes = []string{"pve2", "pve2", "missing"}
	cfg.Topology.ControlPlane = config.NodeConfig{Count: 3, CPU: 8, MemoryMB: 32768}
	cfg.Topology.Workers = config.NodeConfig{Count: 3, CPU: 4, MemoryMB: 16384}
	snapshot := &WizardCapacitySnapshot{discovery: &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "pve1", Status: "online", CPUs: 24, CPUsKnown: true, MemGB: 96, MemKnown: true},
		{Name: "pve2", Status: "online", CPUs: 24, CPUsKnown: true, MemGB: 96, MemKnown: true},
	}}}
	s := NewReviewStep()
	s.SetConfig(cfg)
	s.SetCapacity(snapshot)

	frame := s.View(120, 100)
	tuitest.AssertFits(t, frame, 120, 100)
	out := tuitest.StripANSI(frame)
	for _, want := range []string{"selected capacity", "pve1 over capacity", "pve2 fits", "missing unavailable"} {
		if !strings.Contains(out, want) {
			t.Errorf("review omitted capacity status %q:\n%s", want, out)
		}
	}
}

func TestReviewStep_PreviewShowsRedactedInstallConfigAndEscReturns(t *testing.T) {
	cfg := reviewTestConfig()
	cfg.Cluster.Name = "qa-cluster"
	cfg.Cluster.Domain = "example.test"
	cfg.Provider.Proxmox.Password.Set("never-render-this-password")
	t.Cleanup(cfg.Provider.Proxmox.Password.Zeroize)
	cfg.Provider.Proxmox.APIToken.Set("never-render-this-token")
	t.Cleanup(cfg.Provider.Proxmox.APIToken.Zeroize)
	s := NewReviewStep()
	s.SetConfig(cfg)

	_, _ = s.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if !s.showPreview {
		t.Fatal("p did not open the install-config preview")
	}
	frame := s.View(100, 100)
	tuitest.AssertFits(t, frame, 100, 100)
	out := tuitest.StripANSI(frame)
	for _, want := range []string{"INSTALL-CONFIG PREVIEW", "qa-cluster", "[redacted]"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview omitted %q:\n%s", want, out)
		}
	}
	for _, secret := range []string{"never-render-this-password", "never-render-this-token"} {
		if strings.Contains(out, secret) {
			t.Errorf("preview exposed secret %q", secret)
		}
	}
	if !s.InterceptBack() || s.showPreview {
		t.Fatal("esc did not close the preview without leaving the review step")
	}
}

func TestReviewPlanIncludesHeadlessCommandAndConfigPath(t *testing.T) {
	s := NewReviewStep()
	s.SetConfig(reviewTestConfig())
	s.SetConfigPath("/tmp/qa cluster.yaml")
	frame := tuitest.StripANSI(s.PaneContent(70, 30))
	tuitest.AssertFits(t, frame, 70, 30)
	for _, want := range []string{"DEPLOY PLAN", "WRITES", "/tmp/qa", "HEADLESS", `--config '/tmp/qa cluster.yaml'`, "--confirm-cluster"} {
		if !strings.Contains(frame, want) {
			t.Errorf("review pane omitted %q:\n%s", want, frame)
		}
	}
}

func TestReviewHeadlessCommandShellQuotesArguments(t *testing.T) {
	got := reviewHeadlessCommand("/tmp/O'Brien config.yaml", "qa cluster")
	if want := `--config '/tmp/O'\''Brien config.yaml' --yes --confirm-cluster 'qa cluster'`; !strings.Contains(got, want) {
		t.Fatalf("reviewHeadlessCommand() = %q, want shell-safe arguments containing %q", got, want)
	}
}

func TestReviewStep_HiddenStepGetsNoIndex(t *testing.T) {
	s := NewReviewStep()
	s.SetConfig(reviewTestConfig())

	// files omitted, simulating a step hidden by ShouldShow — header must render without a digit.
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDBasics, Digit: 1},
		{StepID: wizard.StepIDProxmox, Digit: 2},
	})

	out := s.View(100, 100)
	if !strings.Contains(out, "[2] proxmox") {
		t.Error("View() dropped the jump index for a mapped step")
	}
	if strings.Contains(out, "] files & ignition") {
		t.Error("View() shows a jump index for files & ignition, want none (hidden step)")
	}
	if !strings.Contains(out, "files & ignition") {
		t.Error("View() dropped the files & ignition header entirely, want header without an index")
	}
}

func TestReviewStep_DigitKeyEmitsJumpToStepMsg(t *testing.T) {
	s := NewReviewStep()
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDProxmox, Digit: 2},
	})

	_, cmd := s.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if cmd == nil {
		t.Fatal("Update(digit 2) returned a nil cmd, want a JumpToStepMsg command")
	}
	msg := cmd()
	jump, ok := msg.(wizard.JumpToStepMsg)
	if !ok {
		t.Fatalf("Update(digit 2) cmd produced %T, want wizard.JumpToStepMsg", msg)
	}
	if jump.StepID != wizard.StepIDProxmox {
		t.Errorf("JumpToStepMsg.StepID = %v, want StepIDProxmox", jump.StepID)
	}
}

func TestReviewStep_UnmappedDigitDoesNotJump(t *testing.T) {
	s := NewReviewStep()
	s.SetJumpTargets([]wizard.JumpTarget{
		{StepID: wizard.StepIDProxmox, Digit: 2},
	})

	// Digit 9 has no target; the keypress must fall through, not be swallowed.
	_, cmd := s.Update(tea.KeyPressMsg{Code: '9', Text: "9"})
	if cmd == nil {
		return
	}
	if _, ok := cmd().(wizard.JumpToStepMsg); ok {
		t.Fatal("Update(unmapped digit) produced a JumpToStepMsg, want none")
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
		if b.Help == wizard.HelpJump {
			found = true
		}
	}
	if !found {
		t.Error("ShortHelp() does not advertise jump once targets are set")
	}
}

// reviewFullJumpTargets is the digit assignment the wizard computes when
// every reviewJumpOrder step is present (nothing hidden by ShouldShow).
func reviewFullJumpTargets() []wizard.JumpTarget {
	targets := make([]wizard.JumpTarget, len(reviewJumpOrder))
	for i, id := range reviewJumpOrder {
		targets[i] = wizard.JumpTarget{StepID: id, Digit: i + 1}
	}
	return targets
}

func TestReviewStep_LegendDigitsContiguousWhenAddonsHidden(t *testing.T) {
	cfg := reviewTestConfig()
	cfg.Addons = map[string]config.AddonConfig{"flux": {Enabled: false}}

	s := NewReviewStep()
	s.SetConfig(cfg)
	s.SetJumpTargets(reviewFullJumpTargets())

	out := s.View(100, 100)
	if !strings.Contains(out, "[7] advanced") {
		t.Error("View() did not renumber advanced to [7] once the hidden addons section closed the gap")
	}
	if strings.Contains(out, "[8]") {
		t.Error("View() still shows digit 8, want the legend compacted to 1-7")
	}
	if strings.Contains(out, "] addons") {
		t.Error("View() shows a jump index for addons, want the section (and its digit) absent entirely")
	}
}

func TestReviewStep_PinnedFooterShowsActions(t *testing.T) {
	s := NewReviewStep()

	if got := s.PinnedFooter(80); got != "" {
		t.Errorf("PinnedFooter() before SetConfig = %q, want empty", got)
	}

	s.SetConfig(reviewTestConfig())
	footer := s.PinnedFooter(80)
	if !strings.Contains(footer, "deploy now") || !strings.Contains(footer, "save and exit") {
		t.Errorf("PinnedFooter() = %q, want both actions", footer)
	}
}

func TestReviewStep_LongLabelNotClipped(t *testing.T) {
	cfg := reviewTestConfig()
	cfg.Disks.ControlPlaneDataSizeGB = 50
	cfg.Topology.ControlPlane.Count = 3

	s := NewReviewStep()
	s.SetConfig(cfg)
	out := tuitest.StripANSI(s.View(80, 100))

	idx := strings.Index(out, "control plane data disk")
	if idx == -1 {
		t.Fatal("View() dropped the control plane data disk label entirely")
	}
	line := out[idx:]
	if nl := strings.IndexByte(line, '\n'); nl != -1 {
		line = line[:nl]
	}
	if !strings.Contains(line, "gb per control plane node") {
		t.Errorf("control plane data disk value is not on the label's line, got %q", line)
	}
}

func TestReviewStep_BodyHasNoActionRadio(t *testing.T) {
	s := NewReviewStep()
	s.SetConfig(reviewTestConfig())

	out := s.View(100, 100)
	if strings.Contains(out, "deploy now") || strings.Contains(out, "save and exit") {
		t.Error("View() body still renders the action radio, want it pinned to the footer only")
	}
}
