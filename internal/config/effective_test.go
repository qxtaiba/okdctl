package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEffectiveYAMLResourcesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, bootstrap string
		cpu, memory     int
	}{
		{"omitted", "", 12, 32768},
		{"zero", "  bootstrap: {cpu: 0, memory_mb: 0, disk_gb: 0}\n", 12, 32768},
		{"explicit", "  bootstrap: {cpu: 6, memory_mb: 16384, disk_gb: 100}\n", 6, 16384},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			body := fmt.Sprintf("schemaVersion: %s\ntopology:\n  control_plane: {cpu: 12, memory_mb: 32768, disk_gb: 100}\n%snetworking:\n  machine_cidr: 10.0.0.0/20\n", SchemaVersionCurrent, tc.bootstrap)
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			loader := NewLoader()
			cfg, err := loader.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if b := cfg.Topology.Bootstrap; b.CPU != tc.cpu || b.MemoryMB != tc.memory || b.DiskGB != 100 {
				t.Fatalf("bootstrap: %+v", b)
			}
			if cfg.Topology.Workers.DiskGB != 100 || cfg.Networking.StaticIP.Netmask != "255.255.240.0" {
				t.Fatal("derived values differ")
			}
			cfg.Disks.WorkerDataSizeGB = 0
			if err := loader.Save(cfg, path); err != nil {
				t.Fatal(err)
			}
			reloaded, err := loader.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Topology != reloaded.Topology || reloaded.Disks.WorkerDataSizeGB != 0 {
				t.Fatalf("save/load changed effective resources: before=%+v after=%+v disks=%+v", cfg.Topology, reloaded.Topology, reloaded.Disks)
			}
		})
	}
}
