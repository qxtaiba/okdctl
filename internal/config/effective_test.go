package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"
)

// bootstrapBlockPresent reports whether raw YAML has an explicit
// topology.bootstrap block at all (omitzero should drop it entirely once
// every field is zero — a present-but-empty block would still be a bug).
func bootstrapBlockPresent(t *testing.T, raw []byte) bool {
	t.Helper()
	var probe struct {
		Topology struct {
			Bootstrap map[string]any `json:"bootstrap"`
		} `json:"topology"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("parse saved yaml: %v", err)
	}
	return probe.Topology.Bootstrap != nil
}

// TestEffectiveResolvesOmittedFields exercises NormalizeTopology/Effective
// directly against in-memory Config values, independent of the loader.
func TestEffectiveResolvesOmittedFields(t *testing.T) {
	for _, tc := range []struct {
		name           string
		bootstrap      NodeConfig
		cpu, mem, disk int
	}{
		{"omitted", NodeConfig{}, 12, 32768, 100},
		{"zero fields", NodeConfig{CPU: 0, MemoryMB: 0, DiskGB: 0}, 12, 32768, 100},
		{"explicit override", NodeConfig{CPU: 6, MemoryMB: 16384, DiskGB: 100}, 6, 16384, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Topology: TopologyConfig{
				ControlPlane: NodeConfig{CPU: 12, MemoryMB: 32768, DiskGB: 100},
				Bootstrap:    tc.bootstrap,
			}}
			resolved := Effective(cfg)
			if b := resolved.Topology.Bootstrap; b.CPU != tc.cpu || b.MemoryMB != tc.mem || b.DiskGB != tc.disk {
				t.Fatalf("bootstrap: %+v", b)
			}
			if resolved.Topology.Workers.DiskGB != 100 {
				t.Fatalf("Workers.DiskGB = %d; want 100 (mirrors control-plane)", resolved.Topology.Workers.DiskGB)
			}
			if cfg.Topology.Bootstrap != tc.bootstrap {
				t.Fatalf("Effective mutated the input cfg: %+v", cfg.Topology.Bootstrap)
			}
		})
	}
}

// TestSaveDoesNotMaterializeDerivedBootstrap is the reviewer's reproduction
// for the critical save-freezes-resolved-values bug: loading a config with
// bootstrap omitted must never cause a later untouched Save to write an
// explicit bootstrap disk, and a control-plane disk change must still be
// reflected in the *effective* bootstrap disk after reload — never frozen
// at whatever the control-plane disk happened to be at the first load.
func TestSaveDoesNotMaterializeDerivedBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := fmt.Sprintf("schemaVersion: %s\ncluster: {name: c, domain: d}\ntopology:\n  control_plane: {count: 1, cpu: 4, memory_mb: 8192, disk_gb: 100}\n  workers: {count: 0}\nnetworking:\n  machine_cidr: 10.0.0.0/20\n  pod_cidr: 10.128.0.0/14\n  service_cidr: 172.30.0.0/16\n  gateway: 10.0.0.1\n", SchemaVersionCurrent)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader()

	cfg, err := loader.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Topology.Bootstrap.DiskGB != 0 {
		t.Fatalf("raw-loaded Bootstrap.DiskGB = %d; want 0 (bootstrap was omitted in the file)", cfg.Topology.Bootstrap.DiskGB)
	}

	// Save the config back UNTOUCHED — this is the operator doing nothing,
	// not an edit. It must not materialize a bootstrap disk value.
	if err := loader.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrapBlockPresent(t, raw) {
		t.Fatalf("saved YAML materialized a bootstrap block; raw yaml:\n%s", raw)
	}

	// Second cycle: simulate 'okdctl node resize' changing the control-plane
	// disk directly on the struct, the way internal/node/runner.go's
	// persistTopology does, then saving again.
	reloaded, err := loader.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile (reload): %v", err)
	}
	reloaded.Topology.ControlPlane.DiskGB = 200
	if err := loader.Save(reloaded, path); err != nil {
		t.Fatalf("Save (after resize): %v", err)
	}

	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrapBlockPresent(t, raw) {
		t.Fatalf("resize save materialized a stale bootstrap block; raw yaml:\n%s", raw)
	}

	final, err := loader.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile (final): %v", err)
	}
	if got := Effective(final).Topology.Bootstrap.DiskGB; got != 200 {
		t.Fatalf("effective Bootstrap.DiskGB = %d; want 200 (must follow control-plane disk)", got)
	}
}
