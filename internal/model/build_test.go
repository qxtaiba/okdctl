package model

import (
	"net/netip"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

const testInfraID = "lab-x7k2q"

func testConfig(workers int) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Workers.Count = workers
	cfg.Nodes = nil
	return cfg
}

func mustBuild(t *testing.T, cfg *config.Config) Inventory {
	t.Helper()
	inv, err := Build(cfg, testInfraID)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return inv
}

func TestBuild_ThreePlusZero(t *testing.T) {
	inv := mustBuild(t, testConfig(0))
	if inv.InfraID != testInfraID {
		t.Errorf("InfraID = %q", inv.InfraID)
	}
	if inv.APIVIP != netip.MustParseAddr("192.168.1.158") || inv.IngressVIP != netip.MustParseAddr("192.168.1.159") {
		t.Errorf("VIPs = %s/%s", inv.APIVIP, inv.IngressVIP)
	}
	want := []NodeSpec{
		{Role: RoleMaster, Index: 0, Name: "lab-master0", VMID: 6010, MAC: "02:12:83:00:17:7a", IP: netip.MustParseAddr("192.168.1.150"), Host: "pve", CPU: 4, MemoryMiB: 16384, Disks: []Disk{{SizeGiB: 120, Storage: "local-lvm", Serial: "okd6010d0"}}},
		{Role: RoleMaster, Index: 1, Name: "lab-master1", VMID: 6011, MAC: "02:12:83:00:17:7b", IP: netip.MustParseAddr("192.168.1.151"), Host: "pve", CPU: 4, MemoryMiB: 16384, Disks: []Disk{{SizeGiB: 120, Storage: "local-lvm", Serial: "okd6011d0"}}},
		{Role: RoleMaster, Index: 2, Name: "lab-master2", VMID: 6012, MAC: "02:12:83:00:17:7c", IP: netip.MustParseAddr("192.168.1.152"), Host: "pve", CPU: 4, MemoryMiB: 16384, Disks: []Disk{{SizeGiB: 120, Storage: "local-lvm", Serial: "okd6012d0"}}},
	}
	if len(inv.Nodes) != len(want) {
		t.Fatalf("Nodes = %+v; want %d masters", inv.Nodes, len(want))
	}
	for i := range want {
		assertNode(t, inv.Nodes[i], want[i])
	}
	if len(inv.Workers()) != 0 || len(inv.Masters()) != 3 {
		t.Errorf("Masters/Workers = %d/%d", len(inv.Masters()), len(inv.Workers()))
	}
}

func TestBuild_ThreePlusTwoWithOverride(t *testing.T) {
	cfg := testConfig(2)
	cfg.Provider.Proxmox.Nodes = []string{"pve1", "pve2"}
	mem := 32768
	cfg.Nodes = map[string]config.NodeOverride{"lab-worker1": {Host: "pve2", MemoryMiB: &mem}}
	inv := mustBuild(t, cfg)

	wantHosts := []string{"pve1", "pve2", "pve1", "pve2", "pve2"}
	for i, n := range inv.Nodes {
		if n.Host != wantHosts[i] {
			t.Errorf("%s host = %q; want %q", n.Name, n.Host, wantHosts[i])
		}
	}
	workers := inv.Workers()
	if len(workers) != 2 {
		t.Fatalf("Workers = %+v", workers)
	}
	assertNode(t, workers[0], NodeSpec{
		Role: RoleWorker, Index: 0, Name: "lab-worker0", VMID: 6100, MAC: "02:12:83:00:17:d4", IP: netip.MustParseAddr("192.168.1.160"), Host: "pve2", CPU: 8, MemoryMiB: 16384,
		Disks: []Disk{{SizeGiB: 120, Storage: "local-lvm", Serial: "okd6100d0"}, {SizeGiB: 500, Storage: "local-lvm", Serial: "okd6100d1", Wipe: true}},
	})
	assertNode(t, workers[1], NodeSpec{
		Role: RoleWorker, Index: 1, Name: "lab-worker1", VMID: 6101, MAC: "02:12:83:00:17:d5", IP: netip.MustParseAddr("192.168.1.161"), Host: "pve2", CPU: 8, MemoryMiB: 32768,
		Disks: []Disk{{SizeGiB: 120, Storage: "local-lvm", Serial: "okd6101d0"}, {SizeGiB: 500, Storage: "local-lvm", Serial: "okd6101d1", Wipe: true}},
	})
}

func TestBuild_WorkerIndexHole(t *testing.T) {
	cfg := testConfig(2)
	cfg.Nodes = map[string]config.NodeOverride{"lab-worker0": {}, "lab-worker2": {}}
	before := mustBuild(t, cfg).Workers()
	if got := indices(before); got != "0,2" {
		t.Fatalf("count 2 indices = %s; want 0,2", got)
	}

	cfg.Workers.Count = 3
	after := mustBuild(t, cfg).Workers()
	if got := indices(after); got != "0,1,2" {
		t.Fatalf("count 3 indices = %s; want 0,1,2", got)
	}
	assertNode(t, after[0], before[0])
	assertNode(t, after[2], before[1])
	if after[1].Name != "lab-worker1" || after[1].VMID != 6101 || after[1].IP != netip.MustParseAddr("192.168.1.161") {
		t.Errorf("filled worker = %+v", after[1])
	}
}

func TestBuild_Overrides(t *testing.T) {
	cfg := testConfig(1)
	cpu := 2
	cfg.Nodes = map[string]config.NodeOverride{
		"lab-master1": {IP: "192.168.1.200", CPU: &cpu},
		"lab-worker0": {Disks: []config.DiskSpec{{SizeGiB: 200, Storage: "fast"}}},
	}
	inv := mustBuild(t, cfg)
	master1, _ := inv.ByName("lab-master1")
	if master1.IP != netip.MustParseAddr("192.168.1.200") || master1.CPU != 2 || master1.MemoryMiB != 16384 {
		t.Errorf("lab-master1 = %+v", master1)
	}
	worker0, _ := inv.ByName("lab-worker0")
	wantDisks := []Disk{{SizeGiB: 200, Storage: "fast", Serial: "okd6100d0"}}
	assertNode(t, worker0, NodeSpec{Role: RoleWorker, Name: "lab-worker0", VMID: 6100, MAC: "02:12:83:00:17:d4", IP: netip.MustParseAddr("192.168.1.160"), Host: "pve", CPU: 8, MemoryMiB: 16384, Disks: wantDisks})
}

func TestBuild_Errors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"vmid above 24 bits", func(c *config.Config) { c.Provider.Proxmox.VMIDBase = 0xFFFFFF }},
		{"ip override outside cidr", func(c *config.Config) {
			c.Nodes = map[string]config.NodeOverride{"lab-master0": {IP: "10.0.0.5"}}
		}},
		{"ip override malformed", func(c *config.Config) {
			c.Nodes = map[string]config.NodeOverride{"lab-master0": {IP: "nope"}}
		}},
		{"node range outside cidr", func(c *config.Config) { c.Networking.NodeIPStart = "192.168.1.254" }},
		{"no proxmox nodes", func(c *config.Config) { c.Provider.Proxmox.Nodes = nil }},
		{"bad machine cidr", func(c *config.Config) { c.Networking.MachineCIDR = "192.168.1.0" }},
		{"bad api vip", func(c *config.Config) { c.Networking.APIVIP = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig(0)
			tc.mutate(cfg)
			if _, err := Build(cfg, testInfraID); err == nil {
				t.Fatal("Build accepted an invalid config")
			}
		})
	}
}

func TestInventory_Lookups(t *testing.T) {
	inv := mustBuild(t, testConfig(2))
	if n, ok := inv.ByName("lab-worker1"); !ok || n.VMID != 6101 {
		t.Errorf("ByName = %+v, %v", n, ok)
	}
	if n, ok := inv.ByVMID(6012); !ok || n.Name != "lab-master2" {
		t.Errorf("ByVMID = %+v, %v", n, ok)
	}
	if n, ok := inv.ByMAC("02:12:83:00:17:D4"); !ok || n.Name != "lab-worker0" {
		t.Errorf("ByMAC = %+v, %v", n, ok)
	}
	if _, ok := inv.ByName("lab-worker9"); ok {
		t.Error("ByName found a missing node")
	}
	if _, ok := inv.ByVMID(1); ok {
		t.Error("ByVMID found a missing node")
	}
	if _, ok := inv.ByMAC("02:00:00:00:00:00"); ok {
		t.Error("ByMAC found a missing node")
	}
}

func assertNode(t *testing.T, got, want NodeSpec) {
	t.Helper()
	if got.Role != want.Role || got.Index != want.Index || got.Name != want.Name || got.VMID != want.VMID || got.MAC != want.MAC ||
		got.IP != want.IP || got.Host != want.Host || got.CPU != want.CPU || got.MemoryMiB != want.MemoryMiB {
		t.Errorf("node = %+v; want %+v", got, want)
	}
	if len(got.Disks) != len(want.Disks) {
		t.Fatalf("%s disks = %+v; want %+v", want.Name, got.Disks, want.Disks)
	}
	for i := range want.Disks {
		if got.Disks[i] != want.Disks[i] {
			t.Errorf("%s disk %d = %+v; want %+v", want.Name, i, got.Disks[i], want.Disks[i])
		}
	}
}

func indices(nodes []NodeSpec) string {
	out := ""
	for i, n := range nodes {
		if i > 0 {
			out += ","
		}
		out += string(rune('0' + n.Index))
	}
	return out
}
