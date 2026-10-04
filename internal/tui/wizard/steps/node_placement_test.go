package steps

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
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

// demoDiscoveryUnknownCapacity is a single-node fixture exercising two
// honest-unknown displays at once: the node's CPU/memory probe came back
// unknown (nodeDisplayOptions renders "?c/?g"), and a cluster-level storage
// pool no online node reports renders "<pool> — capacity unknown"
// (storageDisplayOptions).
func demoDiscoveryUnknownCapacity() *proxmoxDiscovery {
	return &proxmoxDiscovery{
		Nodes: []proxmoxNode{{
			Name: "pve1", Status: "online",
			CPUsKnown: false, MemKnown: false,
			StorageKnown: true,
			Storage:      []proxmoxStorage{{Name: "local-lvm", Content: "images", TotalGB: 500, TotalKnown: true}},
			BridgesKnown: true, Bridges: demoNodeBridges(),
		}},
		Storage: []proxmoxStorage{
			{Name: "local-lvm", Content: "images,rootdir", TotalGB: 500},
			{Name: "orphan-pool", Content: "images", TotalGB: 900},
		},
		Bridges: demoNodeBridges(),
	}
}

// TestGolden_NodePlacementUnknownCapacity pins demoDiscoveryUnknownCapacity
// through the full model (so the viewport, not the bare step, governs
// height), at both the compact and the wide-split tiers.
func TestGolden_NodePlacementUnknownCapacity(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {180, 48}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			tui.SetTerminalWidth(sz.w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			m := newGoldenModel(t)
			_ = tuitest.RenderAt(t, m, sz.w, sz.h)
			m.Update(wizard.JumpToStepMsg{StepID: wizard.StepIDNodePlacement})
			// Selects the orphaned pool so its "capacity unknown" display
			// renders in the collapsed box, not just inside
			// storageDisplayOptions' own return value; clears the default
			// config's stale "pve" bootstrap node so the field falls back
			// to the fixture's actual node instead of an unmatched value.
			px := m.Config().Provider.Proxmox
			px.DataStorage = "orphan-pool"
			px.Node = ""
			gen := m.CurrentStep().(*NodePlacementStep).generation
			m.Update(discoveryCompleteMsg{generation: gen, discovery: demoDiscoveryUnknownCapacity()})

			// Tabs past bridge, additional networks, os storage, and data
			// storage to focus bootstrap, scrolling both capacity-unknown
			// rows into view at the compact tier too.
			tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
			for range 4 {
				m.Update(tabKey)
				m.Update(wizard.FocusChangedMsg{})
			}

			frame := tuitest.RenderAt(t, m, sz.w, sz.h)
			tuitest.Golden(t, fmt.Sprintf("node-placement-unknown-capacity_%dx%d", sz.w, sz.h), frame)
			tuitest.AssertFits(t, frame, sz.w, sz.h)

			plain := tuitest.StripANSI(frame)
			for _, want := range []string{"pve1 — ?c/?g", "orphan-pool — capacity unknown"} {
				if !strings.Contains(plain, want) {
					t.Errorf("view is missing %q:\n%s", want, plain)
				}
			}
		})
	}
}

// TestNodePlacementStep_StaleDiscoverySuccessCannotOverwriteNewer pins the
// request-identity contract: fake request A is issued, a newer request B is
// issued before A replies (the back-then-re-enter case), B's reply lands
// first, and A's now-stale successful reply must not overwrite it.
func TestNodePlacementStep_StaleDiscoverySuccessCannotOverwriteNewer(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()

	s.Init()
	genA := s.generation

	s.Init() // re-entry while A is still in flight
	genB := s.generation
	if genB == genA {
		t.Fatal("re-entry must issue a new generation")
	}

	discA := &proxmoxDiscovery{Nodes: []proxmoxNode{{Name: "stale-node"}}}
	discB := demoDiscovery()

	step, _ := s.Update(discoveryCompleteMsg{generation: genB, discovery: discB})
	s = step.(*NodePlacementStep)
	step, _ = s.Update(discoveryCompleteMsg{generation: genA, discovery: discA})
	s = step.(*NodePlacementStep)

	if s.discovery != discB {
		t.Fatalf("stale reply A overwrote newer reply B: discovery = %+v", s.discovery)
	}
}

// TestNodePlacementStep_StaleDiscoveryErrorCannotOverwriteNewer repeats the
// above with A returning an error instead of a success: the stale failure
// must not replace B's good, newer result.
func TestNodePlacementStep_StaleDiscoveryErrorCannotOverwriteNewer(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()

	s.Init()
	genA := s.generation

	s.Init() // re-entry while A is still in flight
	genB := s.generation

	discB := demoDiscovery()
	step, _ := s.Update(discoveryCompleteMsg{generation: genB, discovery: discB})
	s = step.(*NodePlacementStep)
	step, _ = s.Update(discoveryCompleteMsg{generation: genA, err: errors.New("connection refused")})
	s = step.(*NodePlacementStep)

	if s.discoveryErr != nil {
		t.Fatalf("stale error reply set discoveryErr = %v, want nil", s.discoveryErr)
	}
	if s.discovery != discB {
		t.Fatalf("stale error reply altered discovery: %+v", s.discovery)
	}
}

// TestNodePlacementStep_ReentryAfterSuccessReusesCachedDiscovery pins the
// reuse-on-re-entry decision: once discovery has succeeded, going back and
// re-entering the step must not re-hit the Proxmox API.
func TestNodePlacementStep_ReentryAfterSuccessReusesCachedDiscovery(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()

	cmd := s.Init()
	if cmd == nil {
		t.Fatal("first Init() must fetch")
	}
	step, _ := s.Update(discoveryCompleteMsg{generation: s.generation, discovery: demoDiscovery()})
	s = step.(*NodePlacementStep)
	if s.phase != phasePlacing || s.discoveryErr != nil {
		t.Fatalf("phase=%v discoveryErr=%v after first load, want phasePlacing with no error", s.phase, s.discoveryErr)
	}

	if cmd := s.Init(); cmd != nil {
		t.Fatal("re-entry after a successful discovery re-issued the fetch, want cached reuse")
	}
	if s.phase != phasePlacing {
		t.Fatalf("re-entry after success flipped phase to %v, want it to stay phasePlacing", s.phase)
	}
}

// TestNodePlacementStep_ReentryAfterErrorRefetches pins the other half of
// the reuse decision: a failed attempt is never cached, so re-entering
// after an error automatically retries instead of leaving the step stuck.
func TestNodePlacementStep_ReentryAfterErrorRefetches(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()

	s.Init()
	genA := s.generation
	step, _ := s.Update(discoveryCompleteMsg{generation: genA, err: errors.New("connection refused")})
	s = step.(*NodePlacementStep)
	if s.discoveryErr == nil {
		t.Fatal("expected a load error")
	}

	cmd := s.Init() // re-entry
	if cmd == nil {
		t.Fatal("re-entry after an error must retry, not get stuck")
	}
	genB := s.generation
	if genB == genA {
		t.Fatal("retry on re-entry must issue a new generation")
	}
	step, _ = s.Update(discoveryCompleteMsg{generation: genB, discovery: demoDiscovery()})
	s = step.(*NodePlacementStep)
	if s.discoveryErr != nil {
		t.Fatalf("discoveryErr = %v after re-entry's retry succeeded, want nil", s.discoveryErr)
	}
}

// TestNodePlacementStep_RefreshKeyRefetchesDiscovery pins the explicit
// escape hatch the reuse decision requires: 'r' while placing re-issues
// discovery even though a successful result is cached.
func TestNodePlacementStep_RefreshKeyRefetchesDiscovery(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()

	s.Init()
	step, _ := s.Update(discoveryCompleteMsg{generation: s.generation, discovery: demoDiscovery()})
	s = step.(*NodePlacementStep)
	genBefore := s.generation

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("'r' while placing must re-issue discovery")
	}
	if s.generation == genBefore {
		t.Fatal("'r' must bump the generation")
	}
}

// TestNodePlacementStep_OverlayHelp pins which states feed the "?" overlay's
// footer-silent "r" entry: nil while discovering (the key does nothing
// yet), nil when placing with no built form (the nil-provider error, which
// never calls buildInnerStep so 'r' has nothing to re-fetch into), and
// {r refresh} once placing has a built form — success or a network error
// with its fallback form alike, since Update's phasePlacing branch lets 'r'
// re-fetch either way.
func TestNodePlacementStep_OverlayHelp(t *testing.T) {
	s := NewNodePlacementStep()
	if got := s.OverlayHelp(); got != nil {
		t.Fatalf("OverlayHelp() while discovering = %+v, want nil", got)
	}

	nilProvider := NewNodePlacementStep()
	nilProvider.cfg = &config.Config{}
	nilProvider.Init()
	if got := nilProvider.OverlayHelp(); got != nil {
		t.Fatalf("OverlayHelp() with no built form = %+v, want nil", got)
	}

	s = NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()
	step, _ := s.Update(discoveryCompleteMsg{discovery: demoDiscovery()})
	s = step.(*NodePlacementStep)
	want := wizard.KeyBinding{Key: "r", Help: "refresh"}
	if got := s.OverlayHelp(); len(got) != 1 || got[0] != want {
		t.Fatalf("OverlayHelp() once placing (success) = %+v, want [%+v]", got, want)
	}

	errored := NewNodePlacementStep()
	errored.cfg = newProxmoxTestConfig()
	step, _ = errored.Update(discoveryCompleteMsg{err: errors.New("connection refused")})
	errored = step.(*NodePlacementStep)
	if got := errored.OverlayHelp(); len(got) != 1 || got[0] != want {
		t.Fatalf("OverlayHelp() once placing (network error, fallback form) = %+v, want [%+v]", got, want)
	}
}

// TestNodePlacementStep_HelpOverlay_RefreshDiscoverable renders the "?"
// overlay through the full wizard.Model pipeline to prove the placing
// phase's "r refresh" binding — real but footer-silent — is actually
// discoverable there.
func TestNodePlacementStep_HelpOverlay_RefreshDiscoverable(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()
	step, _ := s.Update(discoveryCompleteMsg{discovery: demoDiscovery()})
	s = step.(*NodePlacementStep)

	m := wizard.NewModel([]wizard.WizardStep{s}, s.cfg)
	tuitest.RenderAt(t, m, 100, 30)
	mm, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = mm.(*wizard.Model)

	frame := tuitest.StripANSI(m.View().Content)
	if !strings.Contains(frame, "r refresh") {
		t.Fatalf("placing-phase overlay missing %q:\n%s", "r refresh", frame)
	}
}

// TestNodePlacementStep_HelpOverlay_DiscoveringPhaseListsNoRefresh is the
// negative case: while discovery is still in flight, 'r' does nothing, and
// the overlay must not claim otherwise.
func TestNodePlacementStep_HelpOverlay_DiscoveringPhaseListsNoRefresh(t *testing.T) {
	s := NewNodePlacementStep()
	s.cfg = newProxmoxTestConfig()

	m := wizard.NewModel([]wizard.WizardStep{s}, s.cfg)
	tuitest.RenderAt(t, m, 100, 30)
	mm, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = mm.(*wizard.Model)

	frame := tuitest.StripANSI(m.View().Content)
	if strings.Contains(frame, "refresh") {
		t.Fatalf("discovering-phase overlay must not advertise \"refresh\":\n%s", frame)
	}
}

// TestNodePlacementStep_SanitizesHostileProxmoxText drives a tampered
// Proxmox discovery response — a node, storage pool, and bridge name each
// carrying a CSI sequence that would otherwise clear the screen and move the
// cursor home, the first two moves of forging a line of okdctl-looking
// chrome — through the step's real View path, and separately pins a hostile
// discoveryErr through the same screen's error row. The view legitimately
// carries okdctl's own SGR styling (this step renders with lipgloss's
// default, TTY-detected profile regardless of tui's own tracked profile), so
// the test checks for the exact tampered byte sequence rather than any ESC
// byte: that sequence can only survive if sanitization failed, since no
// legitimate style this step emits coincides with a CSI clear-screen/
// cursor-home pair.
func TestNodePlacementStep_SanitizesHostileProxmoxText(t *testing.T) {
	const payload = "\x1b[2J\x1b[H"

	t.Run("node, storage, bridge, and iso names", func(t *testing.T) {
		const hostileISO = "local:iso/fcos" + payload + ".iso"
		cfg := newProxmoxTestConfig()
		cfg.Topology.ControlPlane.Count = 1
		cfg.Provider.Proxmox.FCOSIso = hostileISO // pre-selects it as the field's current value
		s := NewNodePlacementStep()
		s.cfg = cfg

		disc := &proxmoxDiscovery{
			Nodes: []proxmoxNode{{
				Name: "pve1" + payload, Status: "online",
				CPUsKnown: true, CPUs: 4, MemKnown: true, MemGB: 8,
				StorageKnown: true,
				Storage:      []proxmoxStorage{{Name: "local-lvm" + payload, Content: "images", TotalKnown: true, TotalGB: 100}},
			}},
			Storage: []proxmoxStorage{{Name: "local-lvm" + payload, Content: "images"}},
			Bridges: []proxmoxBridge{{Name: "vmbr0" + payload}},
			ISOs:    []string{hostileISO},
		}
		step, _ := s.Update(discoveryCompleteMsg{discovery: disc})
		s = step.(*NodePlacementStep)

		view := s.View(100, 40)
		if strings.Contains(view, payload) {
			t.Fatalf("rendered view carries the raw clear-screen/cursor-home payload:\n%q", view)
		}
		if !strings.Contains(view, "�") {
			t.Fatalf("rendered view shows no sanitization marker for the tampered names:\n%q", view)
		}
	})

	t.Run("discovery error", func(t *testing.T) {
		s := NewNodePlacementStep()
		s.cfg = newProxmoxTestConfig()
		s.phase = phasePlacing
		s.discoveryErr = errors.New("dial proxmox: " + payload)

		view := s.View(100, 40)
		if strings.Contains(view, payload) {
			t.Fatalf("rendered view carries the raw clear-screen/cursor-home payload:\n%q", view)
		}
		if !strings.Contains(view, "�") {
			t.Fatalf("rendered view shows no sanitization marker for the tampered error:\n%q", view)
		}
	})
}
