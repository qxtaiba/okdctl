package setup

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestGetDiskSizes(t *testing.T) {
	cases := []struct {
		name       string
		cpDisk     int
		workerDisk int
		wantCPOS   int
		wantWOS    int
	}{
		{name: "both set", cpDisk: 80, workerDisk: 60, wantCPOS: 80, wantWOS: 60},
		{name: "worker disk zero inherits cp disk", cpDisk: 80, workerDisk: 0, wantCPOS: 80, wantWOS: 80},
		{name: "cp disk zero defaults to 50", cpDisk: 0, workerDisk: 0, wantCPOS: 50, wantWOS: 50},
		{name: "cp disk zero worker disk set", cpDisk: 0, workerDisk: 40, wantCPOS: 50, wantWOS: 40},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Topology: config.TopologyConfig{
					ControlPlane: config.NodeConfig{DiskGB: tt.cpDisk},
					Workers:      config.NodeConfig{DiskGB: tt.workerDisk},
				},
			}
			d := getDiskSizes(cfg)
			if d.cpOS != tt.wantCPOS {
				t.Errorf("cpOS = %d, want %d", d.cpOS, tt.wantCPOS)
			}
			if d.workerOS != tt.wantWOS {
				t.Errorf("workerOS = %d, want %d", d.workerOS, tt.wantWOS)
			}
		})
	}
}

func TestGetBootstrapResources(t *testing.T) {
	cases := []struct {
		name    string
		bsCPU   int
		bsMem   int
		cpCPU   int
		cpMem   int
		wantCPU int
		wantMem int
	}{
		{name: "bootstrap explicit", bsCPU: 8, bsMem: 16384, cpCPU: 4, cpMem: 8192, wantCPU: 8, wantMem: 16384},
		{name: "bootstrap inherits control-plane", bsCPU: 0, bsMem: 0, cpCPU: 4, cpMem: 8192, wantCPU: 4, wantMem: 8192},
		{name: "bootstrap cpu only falls back", bsCPU: 0, bsMem: 32768, cpCPU: 6, cpMem: 8192, wantCPU: 6, wantMem: 32768},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Topology: config.TopologyConfig{
					Bootstrap:    config.NodeConfig{CPU: tt.bsCPU, MemoryMB: tt.bsMem},
					ControlPlane: config.NodeConfig{CPU: tt.cpCPU, MemoryMB: tt.cpMem},
				},
			}
			cpu, mem := getBootstrapResources(cfg)
			if cpu != tt.wantCPU {
				t.Errorf("cpu = %d, want %d", cpu, tt.wantCPU)
			}
			if mem != tt.wantMem {
				t.Errorf("mem = %d, want %d", mem, tt.wantMem)
			}
		})
	}
}

func TestFormatStringList(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  string
	}{
		{name: "empty", input: nil, want: "[]"},
		{name: "empty slice", input: []string{}, want: "[]"},
		{name: "one item", input: []string{"pve1"}, want: `["pve1"]`},
		{name: "two items", input: []string{"pve1", "pve2"}, want: `["pve1", "pve2"]`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatStringList(tt.input); got != tt.want {
				t.Errorf("formatStringList(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatAdditionalNetworks(t *testing.T) {
	cases := []struct {
		name     string
		networks []config.AdditionalNetwork
		want     string
	}{
		{name: "empty", networks: nil, want: "[]"},
		{name: "no vlan tag", networks: []config.AdditionalNetwork{{Bridge: "vmbr1", Model: "virtio"}}, want: `[{ model = "virtio", bridge = "vmbr1" }]`},
		{name: "model defaults to virtio", networks: []config.AdditionalNetwork{{Bridge: "vmbr1"}}, want: `[{ model = "virtio", bridge = "vmbr1" }]`},
		{name: "with vlan tag", networks: []config.AdditionalNetwork{{Bridge: "vmbr1", Model: "e1000", VLANTag: 100}}, want: `[{ model = "e1000", bridge = "vmbr1", tag = 100 }]`},
		{name: "vlan tag zero omitted", networks: []config.AdditionalNetwork{{Bridge: "vmbr2", Model: "virtio", VLANTag: 0}}, want: `[{ model = "virtio", bridge = "vmbr2" }]`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatAdditionalNetworks(tt.networks); got != tt.want {
				t.Errorf("formatAdditionalNetworks() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildTerraformVarsData_threeMastersTwoWorkers(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.ClusterConfig{Name: "mycluster"},
		Provider: config.ProviderConfig{
			Proxmox: &config.ProxmoxConfig{ISOStorage: "iso-store"},
		},
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{Count: 3, CPU: 4, MemoryMB: 16384, DiskGB: 120},
			Workers:      config.NodeConfig{Count: 2, CPU: 8, MemoryMB: 32768, DiskGB: 200},
			Bootstrap:    config.NodeConfig{CPU: 0, MemoryMB: 0},
		},
	}

	got := buildTerraformVarsData(cfg)

	if got.ClusterName != "mycluster" {
		t.Errorf("ClusterName = %q, want %q", got.ClusterName, "mycluster")
	}
	if got.MasterCount != 3 {
		t.Errorf("MasterCount = %d, want 3", got.MasterCount)
	}
	if got.WorkerCount != 2 {
		t.Errorf("WorkerCount = %d, want 2", got.WorkerCount)
	}
	if got.MasterOSDiskSizeGB != 120 {
		t.Errorf("MasterOSDiskSizeGB = %d, want 120", got.MasterOSDiskSizeGB)
	}
	if got.WorkerOSDiskSizeGB != 200 {
		t.Errorf("WorkerOSDiskSizeGB = %d, want 200", got.WorkerOSDiskSizeGB)
	}
	// bootstrap inherits control-plane when unset
	if got.BootstrapCPUCores != 4 {
		t.Errorf("BootstrapCPUCores = %d, want 4 (inherited)", got.BootstrapCPUCores)
	}
	if got.BootstrapMemoryMB != 16384 {
		t.Errorf("BootstrapMemoryMB = %d, want 16384 (inherited)", got.BootstrapMemoryMB)
	}
	if got.CPUType != DefaultProxmoxCPUType {
		t.Errorf("CPUType = %q, want %q (default)", got.CPUType, DefaultProxmoxCPUType)
	}
	wantMasterNames := `"mycluster-master0", "mycluster-master1", "mycluster-master2"`
	if got.MasterNames != wantMasterNames {
		t.Errorf("MasterNames = %q, want %q", got.MasterNames, wantMasterNames)
	}
	wantWorkerNames := `"mycluster-worker0", "mycluster-worker1"`
	if got.WorkerNames != wantWorkerNames {
		t.Errorf("WorkerNames = %q, want %q", got.WorkerNames, wantWorkerNames)
	}
}

func TestBuildTerraformVarsData_workerDiskFallsBackToCPDisk(t *testing.T) {
	cfg := &config.Config{
		Cluster:  config.ClusterConfig{Name: "fallback"},
		Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{ISOStorage: "iso"}},
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{Count: 3, CPU: 4, MemoryMB: 8192, DiskGB: 100},
			Workers:      config.NodeConfig{Count: 2, CPU: 4, MemoryMB: 8192, DiskGB: 0},
		},
	}

	got := buildTerraformVarsData(cfg)
	if got.WorkerOSDiskSizeGB != 100 {
		t.Errorf("WorkerOSDiskSizeGB = %d, want 100 (inherited from cp disk)", got.WorkerOSDiskSizeGB)
	}
}
