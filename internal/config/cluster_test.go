package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSave_RoundTrip(t *testing.T) {
	withOverrides := DefaultConfig()
	cpu, mem, wipe := 8, 32768, true
	withOverrides.Nodes = map[string]NodeOverride{
		"lab-master0": {MemoryMiB: &mem},
		"lab-worker3": {CPU: &cpu, Host: "pve2", IP: "192.168.1.170", Disks: []DiskSpec{{SizeGiB: 120}, {SizeGiB: 500, Storage: "local-lvm", Wipe: &wipe}}},
	}
	withOverrides.Lifecycle.BeforeNextNode = Hook{Command: "./hook.sh", Timeout: 90 * time.Second}

	cases := []struct {
		name string
		cfg  *Config
	}{
		{"default", DefaultConfig()},
		{"minimal", MinimalConfig()},
		{"with overrides", withOverrides},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "okdctl.yaml")
			loader := NewLoader()
			if err := loader.Save(tc.cfg, path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if perm := fi.Mode().Perm(); perm != 0o600 {
				t.Errorf("Save perm = %#o; want 0o600", perm)
			}

			loaded, err := loader.LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile after Save: %v", err)
			}
			if !reflect.DeepEqual(loaded, tc.cfg) {
				t.Errorf("round trip changed the config\n got: %+v\nwant: %+v", loaded, tc.cfg)
			}
		})
	}
}

func TestSave_OmitsEmptyNTP(t *testing.T) {
	for name, ntp := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "okdctl.yaml")
			cfg := DefaultConfig()
			cfg.Networking.NTP = ntp
			if err := NewLoader().Save(cfg, path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "ntp") {
				t.Errorf("Save wrote an ntp key for an empty list:\n%s", data)
			}
		})
	}
}

func TestSave_FillsSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okdctl.yaml")
	cfg := MinimalConfig()
	cfg.SchemaVersion = ""

	if err := NewLoader().Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := NewLoader().LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if loaded.SchemaVersion != SchemaVersionCurrent || SchemaVersionCurrent != "v3" {
		t.Errorf("SchemaVersion = %q; want v3", loaded.SchemaVersion)
	}
}

func TestDefaultConfig_MeetsFloors(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ControlPlane.CPU < 4 || cfg.ControlPlane.MemoryMiB < 16384 || cfg.ControlPlane.Disks[0].SizeGiB < 100 {
		t.Errorf("ControlPlane = %+v; below the master floor", cfg.ControlPlane)
	}
	if cfg.Workers.CPU < 2 || cfg.Workers.MemoryMiB < 8192 || cfg.Workers.Disks[0].SizeGiB < 100 {
		t.Errorf("Workers = %+v; below the worker floor", cfg.Workers)
	}
	if cfg.Provider.Proxmox.CPUType != "host" {
		t.Errorf("CPUType = %q; want host", cfg.Provider.Proxmox.CPUType)
	}
	if MinimalConfig().Workers.Count != 0 {
		t.Errorf("MinimalConfig workers = %d; want 0", MinimalConfig().Workers.Count)
	}
}
