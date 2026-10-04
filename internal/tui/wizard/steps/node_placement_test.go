package steps

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

func newProxmoxTestConfig() *config.Config {
	return &config.Config{
		Provider: config.ProviderConfig{
			Type:    config.ProviderProxmox,
			Proxmox: &config.ProxmoxConfig{},
		},
	}
}

func TestNodePlacementApplyWritesFieldsInIndexOrder(t *testing.T) {
	nodeNames := []string{"pve1", "pve2", "pve3"}
	cfg := newProxmoxTestConfig()
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2

	s := NewNodePlacementStep()
	s.cfg = cfg
	s.buildInnerStep(nil, nodeNames)

	if len(s.controlPlaneFields) != 3 || len(s.workerFields) != 2 {
		t.Fatalf("built %d control-plane / %d worker fields, want 3/2",
			len(s.controlPlaneFields), len(s.workerFields))
	}

	// Non-identity assignment: a reorder bug in Apply would not survive this.
	wantControlPlane := []string{"pve3", "pve1", "pve2"}
	wantWorkers := []string{"pve2", "pve3"}
	for i, v := range wantControlPlane {
		s.controlPlaneFields[i].SetValue(v)
	}
	for i, v := range wantWorkers {
		s.workerFields[i].SetValue(v)
	}

	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got := cfg.Provider.Proxmox.ControlPlaneNodes; !slices.Equal(got, wantControlPlane) {
		t.Errorf("ControlPlaneNodes = %v, want %v", got, wantControlPlane)
	}
	if got := cfg.Provider.Proxmox.WorkerNodes; !slices.Equal(got, wantWorkers) {
		t.Errorf("WorkerNodes = %v, want %v", got, wantWorkers)
	}
	if got := cfg.Provider.Proxmox.Node; got != "pve1" {
		t.Errorf("bootstrap Node = %q, want pve1 (default)", got)
	}
}

// TestNodePlacementStep_NilProviderSettlesIntoError pins bug 33: a nil
// Provider.Proxmox must not leave the step spinning "discovering…" forever
// with no tick, error, or retry.
func TestNodePlacementStep_NilProviderSettlesIntoError(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = &config.Config{}

	if cmd := s.Init(); cmd != nil {
		t.Fatal("Init with no provider returned a command, want none")
	}
	view := s.View(100, 30)
	if strings.Contains(view, "discovering") {
		t.Fatalf("View still claims to be discovering:\n%s", view)
	}
	if !strings.Contains(view, "proxmox") {
		t.Fatalf("View carries no explanatory error:\n%s", view)
	}
}

// TestNodePlacementStep_HeterogeneousClusterWarns pins bug 10's UI half:
// when discovery found differing per-node inventories, the placement header
// says so, since the pick lists show only what every online node shares.
func TestNodePlacementStep_HeterogeneousClusterWarns(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()

	disc := demoDiscovery()
	disc.Heterogeneous = true
	step, _ := s.Update(discoveryCompleteMsg{discovery: disc})
	s = step.(*NodePlacementStep)

	view := s.View(100, 30)
	if !strings.Contains(view, "differ") {
		t.Fatalf("View() carries no heterogeneity warning:\n%s", view)
	}

	disc = demoDiscovery()
	step, _ = s.Update(discoveryCompleteMsg{discovery: disc})
	s = step.(*NodePlacementStep)
	if view := s.View(100, 30); strings.Contains(view, "differ") {
		t.Fatalf("homogeneous View() carries a warning:\n%s", view)
	}
}

func TestNodePlacementCapacityLabelsAndDemandFollowSelection(t *testing.T) {
	cfg := newProxmoxTestConfig()
	cfg.Topology.ControlPlane = config.NodeConfig{Count: 1, CPU: 4, MemoryMB: 8192, DiskGB: 50}
	cfg.Topology.Bootstrap = config.NodeConfig{Count: 1, CPU: 4, MemoryMB: 8192, DiskGB: 50}
	s := NewNodePlacementStep()
	s.cfg = cfg
	disc := &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "pve1", Status: "online", CPUs: 4, CPUsKnown: true, MemGB: 8, MemKnown: true},
		{Name: "pve2", Status: "offline", CPUs: 8, CPUsKnown: true, MemGB: 16, MemKnown: true},
	}}
	step, _ := s.Update(discoveryCompleteMsg{discovery: disc})
	s = step.(*NodePlacementStep)
	initial := s.View(120, 40)
	plainInitial := tuitest.StripANSI(initial)
	for _, want := range []string{"pve1", "online", "4c", "8g", "oversubscribed", "pve2 offline · 8c · 16g"} {
		if !strings.Contains(plainInitial, want) {
			t.Errorf("initial view does not contain %q:\n%s", want, initial)
		}
	}
	if got := tuitest.StripANSI(strings.Join(nodeDisplayOptions(disc.Nodes, []string{"pve2"}), "")); got != "pve2 — 8c/16g offline" {
		t.Errorf("offline node option = %q, want annotated value", got)
	}

	_ = s.bootstrapField.Focus()
	s.bootstrapField.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	updated := s.View(120, 40)
	if !strings.Contains(updated, "pve2: 4c/8g/50gb") {
		t.Fatalf("updated view has no reassigned bootstrap demand:\n%s", updated)
	}
	if strings.Contains(updated, "oversubscribed") {
		t.Fatalf("updated view still marks a host oversubscribed:\n%s", updated)
	}
	if got := s.bootstrapField.Value(); got != "pve2" {
		t.Fatalf("selected node value = %q, want saved value pve2", got)
	}
}

// TestNodePlacementAssignmentDemandElidesVisiblyAtNarrowWidth pins the width
// idiom on the per-node demand row: several long node names must overflow a
// narrow row into a single, visibly elided ("…") line rather than silently
// wrapping and dropping a trailing node's demand, which the forbidden bare
// Width-based render used to do.
func TestNodePlacementAssignmentDemandElidesVisiblyAtNarrowWidth(t *testing.T) {
	cfg := newProxmoxTestConfig()
	cfg.Topology.ControlPlane = config.NodeConfig{Count: 3, CPU: 4, MemoryMB: 8192, DiskGB: 50}
	cfg.Topology.Workers = config.NodeConfig{Count: 1, CPU: 2, MemoryMB: 4096, DiskGB: 20}
	cfg.Provider.Proxmox.ControlPlaneNodes = []string{
		"homelab-rack-a-master-node-001",
		"homelab-rack-b-master-node-002",
		"homelab-rack-c-master-node-003",
	}
	cfg.Provider.Proxmox.WorkerNodes = []string{"homelab-rack-a-worker-node-001"}

	s := NewNodePlacementStep()
	s.cfg = cfg
	disc := &proxmoxDiscovery{Nodes: []proxmoxNode{
		{Name: "homelab-rack-a-master-node-001", Status: "online", CPUs: 2, CPUsKnown: true, MemGB: 4, MemKnown: true},
		{Name: "homelab-rack-b-master-node-002", Status: "online", CPUs: 8, CPUsKnown: true, MemGB: 16, MemKnown: true},
		{Name: "homelab-rack-c-master-node-003", Status: "online", CPUs: 8, CPUsKnown: true, MemGB: 16, MemKnown: true},
		{Name: "homelab-rack-a-worker-node-001", Status: "online", CPUs: 8, CPUsKnown: true, MemGB: 16, MemKnown: true},
	}}
	step, _ := s.Update(discoveryCompleteMsg{discovery: disc})
	s = step.(*NodePlacementStep)

	demand := s.assignmentDemand(78)
	if got := lipgloss.Width(demand); got > 78 {
		t.Fatalf("demand row is %d columns wide, want <= 78: %q", got, tuitest.StripANSI(demand))
	}
	if strings.Contains(demand, "\n") {
		t.Fatalf("demand row wrapped onto multiple lines instead of eliding:\n%s", tuitest.StripANSI(demand))
	}
	plain := tuitest.StripANSI(demand)
	if !strings.Contains(plain, "assigned demand") {
		t.Fatalf("demand row lost its own label: %q", plain)
	}
	if !strings.HasSuffix(strings.TrimRight(plain, " "), "…") {
		t.Fatalf("overflowing demand row has no visible elision marker: %q", plain)
	}

	view := tuitest.StripANSI(s.View(80, 40))
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > 80 {
			t.Fatalf("view line is %d columns wide, want <= 80: %q", got, line)
		}
	}
}

func TestNodePlacementStep_DiscoveryTransitionsToPlacingPhase(t *testing.T) {
	cfg := newProxmoxTestConfig()
	cfg.Topology.ControlPlane.Count = 1
	cfg.Topology.Workers.Count = 1

	s := NewNodePlacementStep()
	s.cfg = cfg

	if s.phase != phaseDiscovering {
		t.Fatalf("initial phase = %v, want phaseDiscovering", s.phase)
	}
	if s.inner != nil {
		t.Fatal("inner form built before discovery completes")
	}

	disc := &proxmoxDiscovery{Nodes: []proxmoxNode{{Name: "pve1"}, {Name: "pve2"}}}
	_, _ = s.Update(discoveryCompleteMsg{discovery: disc})

	if s.phase != phasePlacing {
		t.Fatalf("phase after discoveryCompleteMsg = %v, want phasePlacing", s.phase)
	}
	if s.discovery != disc {
		t.Error("s.discovery was not set from the discoveryCompleteMsg payload")
	}
	if s.inner == nil {
		t.Fatal("inner form is nil after discovery completes")
	}
}

func TestNodePlacementStep_TabAdvancesAcrossSections(t *testing.T) {
	cfg := newProxmoxTestConfig()
	cfg.Topology.ControlPlane.Count = 2
	cfg.Topology.Workers.Count = 1

	s := NewNodePlacementStep()
	s.cfg = cfg
	s.buildInnerStep(nil, []string{"pve1", "pve2"})
	s.phase = phasePlacing
	s.SetFocused(true)

	// build order: bootstrap(1), control plane(2), workers(1) — no infra section since disc is nil.
	if got := s.inner.CurrentSection(); got != 0 {
		t.Fatalf("initial CurrentSection() = %d, want 0 (bootstrap)", got)
	}

	tab := tea.KeyPressMsg{Code: tea.KeyTab}

	// bootstrap's only field is also its last: tab must cross to control plane, not wrap.
	_, _ = s.Update(tab)
	if got := s.inner.CurrentSection(); got != 1 {
		t.Fatalf("CurrentSection() after 1st tab = %d, want 1 (control plane)", got)
	}

	// control plane has two fields: the first tab stays within the section.
	_, _ = s.Update(tab)
	if got := s.inner.CurrentSection(); got != 1 {
		t.Fatalf("CurrentSection() mid-section tab = %d, want 1 (still control plane)", got)
	}

	// second tab lands on the section's last field: crosses into workers.
	_, _ = s.Update(tab)
	if got := s.inner.CurrentSection(); got != 2 {
		t.Fatalf("CurrentSection() after crossing control plane = %d, want 2 (workers)", got)
	}

	// workers is the last section: tab at its last field is a bounded no-op.
	_, _ = s.Update(tab)
	if got := s.inner.CurrentSection(); got != 2 {
		t.Fatalf("CurrentSection() at final boundary = %d, want 2 (unchanged)", got)
	}
}

func TestNodePlacementStep_ShortHelpWhileDiscovering(t *testing.T) {
	s := NewNodePlacementStep()
	if s.phase != phaseDiscovering {
		t.Fatalf("initial phase = %v, want phaseDiscovering", s.phase)
	}
	if got := s.ShortHelp(); len(got) == 0 {
		t.Fatal("ShortHelp() while discovering is nil, want {esc back, ctrl+c quit}")
	}
}

// TestNodePlacementStep_ShortHelpEnterLabelIsContinue guards E-C5: node
// placement collects several fields (bridge, storage, per-node role
// assignments) before the wizard can advance — a form step, per
// keymap_help.go's rule, so its enter label must read "continue" like every
// other form step (DataDrivenStep, ParamsStep), not "confirm".
func TestNodePlacementStep_ShortHelpEnterLabelIsContinue(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()
	s.phase = phasePlacing
	s.buildInnerStep(nil, []string{"pve"})

	for _, b := range s.ShortHelp() {
		if b.Key == wizard.HelpEnter {
			if b.Help != wizard.HelpContinue {
				t.Errorf("enter label = %q, want %q", b.Help, wizard.HelpContinue)
			}
			return
		}
	}
	t.Fatal("ShortHelp() while placing carries no enter binding")
}

func TestNodePlacementStep_EnterWithInvalidFieldFocusesAndReportsErrFixHighlighted(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()
	s.phase = phasePlacing

	required := components.NewInputField("test required", "")
	required.Required = true
	s.inner = wizard.NewMultiSectionForm([]wizard.FormSection{
		{Title: "test", Group: components.NewInputGroup(required)},
	})
	s.SetFocused(true)

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update(enter) with an invalid field: want a cmd, got nil")
	}

	errMsg, ok := firstErrorSetMsg(cmd)
	if !ok || !errors.Is(errMsg.Error, wizard.ErrFixHighlighted) {
		t.Fatalf("Update(enter) with an invalid field = %#v, want ErrorSetMsg(ErrFixHighlighted)", cmd())
	}
	if got := s.inner.FocusedField(); got != required {
		t.Fatalf("FocusedField() after enter = %v, want the invalid field focused", got)
	}
}

func TestNodePlacementStep_ShouldShow(t *testing.T) {
	s := NewNodePlacementStep()
	if !s.ShouldShow(newProxmoxTestConfig()) {
		t.Error("ShouldShow(proxmox provider) = false, want true")
	}

	other := NewNodePlacementStep()
	otherCfg := &config.Config{Provider: config.ProviderConfig{Type: config.ProviderType("aws")}}
	if other.ShouldShow(otherCfg) {
		t.Error("ShouldShow(non-proxmox provider) = true, want false")
	}
}

func TestNodePlacementStep_DefaultsRoundTripExistingAssignments(t *testing.T) {
	nodeNames := []string{"pve1", "pve2", "pve3"}
	cfg := newProxmoxTestConfig()
	cfg.Provider.Proxmox.ControlPlaneNodes = []string{"pve2", "pve3", "pve1"}
	cfg.Provider.Proxmox.WorkerNodes = []string{"pve3", "pve2"}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2

	s := NewNodePlacementStep()
	s.cfg = cfg
	s.buildInnerStep(nil, nodeNames)

	wantControlPlane := []string{"pve2", "pve3", "pve1"}
	wantWorkers := []string{"pve3", "pve2"}
	for i, want := range wantControlPlane {
		if got := s.controlPlaneFields[i].Value(); got != want {
			t.Errorf("controlPlaneFields[%d].Value() = %q, want %q (pre-set default)", i, got, want)
		}
	}
	for i, want := range wantWorkers {
		if got := s.workerFields[i].Value(); got != want {
			t.Errorf("workerFields[%d].Value() = %q, want %q (pre-set default)", i, got, want)
		}
	}

	if err := s.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := cfg.Provider.Proxmox.ControlPlaneNodes; !slices.Equal(got, wantControlPlane) {
		t.Errorf("ControlPlaneNodes after Apply = %v, want unchanged %v", got, wantControlPlane)
	}
	if got := cfg.Provider.Proxmox.WorkerNodes; !slices.Equal(got, wantWorkers) {
		t.Errorf("WorkerNodes after Apply = %v, want unchanged %v", got, wantWorkers)
	}
}

func TestParseAdditionalNetworks(t *testing.T) {
	if got := parseAdditionalNetworks("", nil); got != nil {
		t.Fatalf("parseAdditionalNetworks(empty) = %v, want nil", got)
	}
	if got := parseAdditionalNetworks("   ", nil); got != nil {
		t.Fatalf("parseAdditionalNetworks(whitespace) = %v, want nil", got)
	}

	got := parseAdditionalNetworks("vmbr1", nil)
	if len(got) != 1 || got[0] != (config.AdditionalNetwork{Bridge: "vmbr1", Model: "virtio"}) {
		t.Fatalf("parseAdditionalNetworks(vmbr1) = %+v", got)
	}

	got = parseAdditionalNetworks(" vmbr1 , vmbr2 ", nil)
	if len(got) != 2 || got[0].Bridge != "vmbr1" || got[1].Bridge != "vmbr2" {
		t.Fatalf("parseAdditionalNetworks(trimmed list) = %+v", got)
	}

	existing := []config.AdditionalNetwork{{Bridge: "vmbr1", Model: "e1000", VLANTag: 100}}
	got = parseAdditionalNetworks("vmbr1,vmbr2", existing)
	if len(got) != 2 {
		t.Fatalf("parseAdditionalNetworks preserve+new: len = %d, want 2", len(got))
	}
	if got[0] != existing[0] {
		t.Errorf("parseAdditionalNetworks did not preserve existing entry: got %+v, want %+v", got[0], existing[0])
	}
	if got[1].Bridge != "vmbr2" || got[1].Model != "virtio" {
		t.Errorf("parseAdditionalNetworks new entry = %+v, want Bridge=vmbr2 Model=virtio", got[1])
	}
}

func TestAdditionalNetworksBridges(t *testing.T) {
	if got := additionalNetworksBridges(nil); got != "" {
		t.Fatalf("additionalNetworksBridges(nil) = %q, want empty", got)
	}
	nets := []config.AdditionalNetwork{{Bridge: "vmbr1"}, {Bridge: "vmbr2"}}
	if got := additionalNetworksBridges(nets); got != "vmbr1,vmbr2" {
		t.Fatalf("additionalNetworksBridges() = %q, want vmbr1,vmbr2", got)
	}
}

func TestFilterStorageByContent(t *testing.T) {
	storage := []proxmoxStorage{
		{Name: "local", Content: "iso,vztmpl"},
		{Name: "local-lvm", Content: "images,rootdir"},
		{Name: "backup", Content: "backup"},
	}
	got := filterStorageByContent(storage, "images")
	if len(got) != 1 || got[0] != "local-lvm" {
		t.Fatalf("filterStorageByContent(images) = %v, want [local-lvm]", got)
	}
	got = filterStorageByContent(storage, "iso")
	if len(got) != 1 || got[0] != "local" {
		t.Fatalf("filterStorageByContent(iso) = %v, want [local]", got)
	}
}

func TestFirstMatch(t *testing.T) {
	options := []string{"a", "b", "c"}

	if got := firstMatch(options, "b", "a"); got != "b" {
		t.Errorf("firstMatch(current present) = %q, want b", got)
	}
	if got := firstMatch(options, "z", "b"); got != "b" {
		t.Errorf("firstMatch(current absent, fallback present) = %q, want b", got)
	}
	if got := firstMatch(options, "z", "z"); got != "a" {
		t.Errorf("firstMatch(neither present) = %q, want first option a", got)
	}
	if got := firstMatch(nil, "z", "z"); got != "" {
		t.Errorf("firstMatch(no options) = %q, want empty", got)
	}
}
