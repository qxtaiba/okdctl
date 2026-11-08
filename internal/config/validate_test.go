package config

import (
	"strings"
	"testing"
)

type finding struct {
	field   string
	message string
}

func assertFinding(t *testing.T, cfg *Config, want finding) {
	t.Helper()
	result := cfg.Validate()
	for _, e := range result.Errors {
		if e.Field == want.field && strings.Contains(e.Message, want.message) {
			return
		}
	}
	t.Errorf("Validate = %v; want %s: ...%s...", result, want.field, want.message)
}

func assertValid(t *testing.T, cfg *Config) {
	t.Helper()
	if result := cfg.Validate(); !result.IsValid() {
		t.Errorf("Validate = %v; want no findings", result)
	}
}

func withOverrides() *Config {
	cfg := DefaultConfig()
	cpu, mem := 8, 32768
	cfg.Provider.Proxmox.Nodes = []string{"pve1", "pve2"}
	cfg.Nodes = map[string]NodeOverride{
		"lab-master0": {MemoryMiB: &mem},
		"lab-worker3": {CPU: &cpu, Host: "pve2", IP: "192.168.1.170", Disks: []DiskSpec{{SizeGiB: 120}, {SizeGiB: 500}}},
	}
	return cfg
}

func TestValidate_AcceptsShippedConfigs(t *testing.T) {
	cases := map[string]*Config{
		"default":        DefaultConfig(),
		"minimal":        MinimalConfig(),
		"with overrides": withOverrides(),
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) { assertValid(t, cfg) })
	}
}

func TestValidate_NilConfig(t *testing.T) {
	var cfg *Config
	assertFinding(t, cfg, finding{"_", "config is nil"})
}

func TestValidate_CollectsEveryFinding(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Cluster.Name = ""
	cfg.Provider.Proxmox.Bridge = ""
	cfg.Networking.HostPrefix = 8
	cfg.ControlPlane.CPU = 1
	result := cfg.Validate()
	fields := map[string]bool{}
	for _, e := range result.Errors {
		fields[e.Field] = true
	}
	for _, want := range []string{FieldClusterName, FieldProxmoxBridge, FieldNetworkingHostPrefix, FieldControlPlane + ".cpu"} {
		if !fields[want] {
			t.Errorf("Validate = %v; missing a finding on %s", result, want)
		}
	}
	if !strings.Contains(result.Error(), "; ") {
		t.Errorf("Error() = %q; want the findings joined", result.Error())
	}
}

func TestValidationResult_Error(t *testing.T) {
	r := &ValidationResult{}
	if r.Error() != "" || !r.IsValid() {
		t.Errorf("empty result: Error() = %q, IsValid() = %v", r.Error(), r.IsValid())
	}
	r.AddError("a.b", "first")
	r.AddError("c", "second")
	if got := r.Error(); got != "a.b: first; c: second" {
		t.Errorf("Error() = %q", got)
	}
}
