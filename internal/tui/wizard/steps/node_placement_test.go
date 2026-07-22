package steps

import (
	"slices"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

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
