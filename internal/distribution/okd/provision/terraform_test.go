package provision

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/workspace"
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
			Proxmox: &config.ProxmoxConfig{ISOStorage: "iso-store", HAEnabled: true},
		},
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{Count: 3, CPU: 4, MemoryMB: 16384, DiskGB: 120},
			Workers:      config.NodeConfig{Count: 2, CPU: 8, MemoryMB: 32768, DiskGB: 200},
			Bootstrap:    config.NodeConfig{CPU: 0, MemoryMB: 0},
		},
	}

	got := buildTerraformVarsData(cfg)

	if got.MasterOSDiskSizeGB != 120 {
		t.Errorf("MasterOSDiskSizeGB = %d, want 120", got.MasterOSDiskSizeGB)
	}
	if got.WorkerOSDiskSizeGB != 200 {
		t.Errorf("WorkerOSDiskSizeGB = %d, want 200", got.WorkerOSDiskSizeGB)
	}
	if got.BootstrapCPUCores != 4 {
		t.Errorf("BootstrapCPUCores = %d, want 4 (inherited)", got.BootstrapCPUCores)
	}
	if got.BootstrapMemoryMB != 16384 {
		t.Errorf("BootstrapMemoryMB = %d, want 16384 (inherited)", got.BootstrapMemoryMB)
	}
	if got.CPUType != DefaultProxmoxCPUType {
		t.Errorf("CPUType = %q, want %q (default)", got.CPUType, DefaultProxmoxCPUType)
	}
	if !got.HAEnabled {
		t.Error("HAEnabled = false, want true (propagated from provider config)")
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

func TestReadTerraformVarsSizing_RoundTripsWrittenVars(t *testing.T) {
	cfg := &config.Config{
		Cluster:  config.ClusterConfig{Name: "sizingcluster"},
		Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{ISOStorage: "iso-store"}},
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{Count: 3, CPU: 4, MemoryMB: 8192, DiskGB: 100},
			Workers:      config.NodeConfig{Count: 2, CPU: 8, MemoryMB: 16384, DiskGB: 200},
		},
	}
	envDir := t.TempDir()
	if err := WriteTerraformVars(cfg, envDir); err != nil {
		t.Fatalf("WriteTerraformVars: %v", err)
	}

	got, found, err := ReadTerraformVarsSizing(envDir)
	if err != nil {
		t.Fatalf("ReadTerraformVarsSizing: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	want := TerraformVarsSizing{
		MasterCPU: 4, MasterMemoryMB: 8192, MasterOSDiskGB: 100,
		WorkerCPU: 8, WorkerMemoryMB: 16384, WorkerOSDiskGB: 200,
	}
	if got != want {
		t.Errorf("ReadTerraformVarsSizing() = %+v, want %+v", got, want)
	}
}

// TestReadTerraformVarsSizing_WorkerDiskInheritsControlPlane locks the same
// worker-disk-falls-back-to-cp-disk inheritance rule getDiskSizes applies at
// write time: when Workers.DiskGB is unset, WriteTerraformVars renders
// worker_os_disk_size_gb equal to the control-plane disk, and the reader
// must round-trip that.
func TestReadTerraformVarsSizing_WorkerDiskInheritsControlPlane(t *testing.T) {
	cfg := &config.Config{
		Cluster:  config.ClusterConfig{Name: "inheritcluster"},
		Provider: config.ProviderConfig{Proxmox: &config.ProxmoxConfig{ISOStorage: "iso-store"}},
		Topology: config.TopologyConfig{
			ControlPlane: config.NodeConfig{Count: 3, CPU: 4, MemoryMB: 8192, DiskGB: 100},
			Workers:      config.NodeConfig{Count: 2, CPU: 8, MemoryMB: 16384, DiskGB: 0},
		},
	}
	envDir := t.TempDir()
	if err := WriteTerraformVars(cfg, envDir); err != nil {
		t.Fatalf("WriteTerraformVars: %v", err)
	}

	got, found, err := ReadTerraformVarsSizing(envDir)
	if err != nil {
		t.Fatalf("ReadTerraformVarsSizing: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if got.WorkerOSDiskGB != 100 {
		t.Errorf("WorkerOSDiskGB = %d, want 100 (inherited from cp disk)", got.WorkerOSDiskGB)
	}
}

func TestReadTerraformVarsSizing_MissingFileIsNotAnError(t *testing.T) {
	_, found, err := ReadTerraformVarsSizing(t.TempDir())
	if err != nil {
		t.Fatalf("want nil error for a missing tfvars file, got %v", err)
	}
	if found {
		t.Error("found = true, want false")
	}
}

func TestWorkerISOsPlanVar(t *testing.T) {
	cases := []struct {
		name        string
		isoStorage  string
		workerCount int
		want        string
	}{
		{name: "single worker", isoStorage: "local", workerCount: 1, want: `["local:iso/worker0.iso"]`},
		{name: "three workers", isoStorage: "iso-store", workerCount: 3, want: `["iso-store:iso/worker0.iso", "iso-store:iso/worker1.iso", "iso-store:iso/worker2.iso"]`},
		{name: "zero workers", isoStorage: "local", workerCount: 0, want: `[]`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorkerISOsPlanVar(tt.isoStorage, tt.workerCount); got != tt.want {
				t.Errorf("WorkerISOsPlanVar(%q, %d) = %q, want %q", tt.isoStorage, tt.workerCount, got, tt.want)
			}
		})
	}
}

func TestReadTerraformVarsSizing_MissingKeyErrors(t *testing.T) {
	envDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(envDir, "terraform.tfvars"), []byte("master_cpu_cores = 4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := ReadTerraformVarsSizing(envDir); err == nil || found {
		t.Fatalf("want (false, error) for a tfvars missing sizing keys, got (found=%v, err=%v)", found, err)
	}
}

func TestWriteTerraformVars_PreservesBootstrapSentinel(t *testing.T) {
	cfg := config.DefaultConfig()
	envDir := t.TempDir()
	sentinel := filepath.Join(envDir, workspace.BootstrapStateSentinelFile)
	sentinelBytes := []byte(`{"bootstrap_enabled": false}`)
	if err := os.WriteFile(sentinel, sentinelBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteTerraformVars(cfg, envDir); err != nil {
		t.Fatalf("WriteTerraformVars: %v", err)
	}

	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("sentinel must survive WriteTerraformVars: %v", err)
	}
	if !bytes.Equal(got, sentinelBytes) {
		t.Errorf("sentinel bytes changed: %q", got)
	}
	fi, err := os.Stat(filepath.Join(envDir, "terraform.tfvars"))
	if err != nil {
		t.Fatalf("terraform.tfvars not rendered: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("terraform.tfvars perm = %#o, want 0600", perm)
	}
}

func TestTerraformVarsWriters_RequireProxmoxProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox = nil
	envDir := t.TempDir()

	if err := WriteTerraformVars(cfg, envDir); err == nil {
		t.Error("WriteTerraformVars without proxmox provider: want error, got nil")
	}
}
