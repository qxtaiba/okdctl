package steps

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

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
