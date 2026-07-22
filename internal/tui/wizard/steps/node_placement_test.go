package steps

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
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
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Type:    config.ProviderProxmox,
			Proxmox: &config.ProxmoxConfig{},
		},
	}
	cfg.Topology.ControlPlane.Count = 3
	cfg.Topology.Workers.Count = 2

	s := NewNodePlacementStep()
	s.cfg = cfg
	s.buildInnerStep(nil, nodeNames)

	if len(s.controlPlaneFields) != 3 || len(s.workerFields) != 2 {
		t.Fatalf("built %d control-plane / %d worker fields, want 3/2",
			len(s.controlPlaneFields), len(s.workerFields))
	}

	// Deliberately non-identity assignment: a reorder bug in Apply would not
	// survive these permuted picks.
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

	// Sections in build order: bootstrap (1 field), control plane (2 fields),
	// workers (1 field) — disc is nil so there is no infrastructure section.
	if got := s.inner.CurrentSection(); got != 0 {
		t.Fatalf("initial CurrentSection() = %d, want 0 (bootstrap)", got)
	}

	tab := tea.KeyPressMsg{Code: tea.KeyTab}

	// bootstrap's only field is also its last field: tab must cross into
	// control plane rather than wrapping within the section.
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

func TestBridgeNames(t *testing.T) {
	bridges := []proxmoxBridge{{Name: "vmbr0"}, {Name: "vmbr1"}}
	got := bridgeNames(bridges)
	if len(got) != 2 || got[0] != "vmbr0" || got[1] != "vmbr1" {
		t.Fatalf("bridgeNames() = %v", got)
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
