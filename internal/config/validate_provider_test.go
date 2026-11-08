package config

import "testing"

func TestValidateProvider(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   finding
	}{
		{"type", func(c *Config) { c.Provider.Type = "vsphere" }, finding{FieldProviderType, "must be proxmox"}},
		{"endpoint required", func(c *Config) { c.Provider.Proxmox.Endpoint = "" }, finding{FieldProxmoxEndpoint, "required"}},
		{"endpoint no scheme", func(c *Config) { c.Provider.Proxmox.Endpoint = "pve:8006" }, finding{FieldProxmoxEndpoint, "URL with scheme and host"}},
		{"endpoint no host", func(c *Config) { c.Provider.Proxmox.Endpoint = "https://" }, finding{FieldProxmoxEndpoint, "URL with scheme and host"}},
		{"no nodes", func(c *Config) { c.Provider.Proxmox.Nodes = nil }, finding{FieldProxmoxNodes, "at least one"}},
		{"blank node", func(c *Config) { c.Provider.Proxmox.Nodes = []string{"pve", ""} }, finding{FieldProxmoxNodes + "[1]", "required"}},
		{"storage", func(c *Config) { c.Provider.Proxmox.Storage = "" }, finding{FieldProxmoxStorage, "required"}},
		{"iso storage", func(c *Config) { c.Provider.Proxmox.ISOStorage = "" }, finding{FieldProxmoxISOStorage, "required"}},
		{"bridge", func(c *Config) { c.Provider.Proxmox.Bridge = "" }, finding{FieldProxmoxBridge, "required"}},
		{"vlan too high", func(c *Config) { c.Provider.Proxmox.VLAN = 4095 }, finding{FieldProxmoxVLAN, "0 (untagged) or 1..4094"}},
		{"vlan negative", func(c *Config) { c.Provider.Proxmox.VLAN = -1 }, finding{FieldProxmoxVLAN, "0 (untagged) or 1..4094"}},
		{"cpu type old model", func(c *Config) { c.Provider.Proxmox.CPUType = "x86-64-v2" }, finding{FieldProxmoxCPUType, "host or x86-64-v3 or newer"}},
		{"cpu type unknown", func(c *Config) { c.Provider.Proxmox.CPUType = "kvm64" }, finding{FieldProxmoxCPUType, "host or x86-64-v3 or newer"}},
		{"vmid base low", func(c *Config) { c.Provider.Proxmox.VMIDBase = 99 }, finding{FieldProxmoxVMIDBase, "at least 100"}},
		{"vmid overflow", func(c *Config) { c.Provider.Proxmox.VMIDBase = 0xFFFFFF - 100 }, finding{FieldProxmoxVMIDBase, "exceeds 16777215"}},
		{
			"vmid overflow via worker index",
			func(c *Config) {
				c.Provider.Proxmox.VMIDBase = 0xFFFFFF - 110
				c.Nodes = map[string]NodeOverride{"lab-worker11": {Host: "pve"}}
			},
			finding{FieldProxmoxVMIDBase, "exceeds 16777215"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mutate(cfg)
			assertFinding(t, cfg, tc.want)
		})
	}

	passing := map[string]func(*Config){
		"x86-64-v3":       func(c *Config) { c.Provider.Proxmox.CPUType = "x86-64-v3" },
		"x86-64-v4":       func(c *Config) { c.Provider.Proxmox.CPUType = "x86-64-v4" },
		"vlan 4094":       func(c *Config) { c.Provider.Proxmox.VLAN = 4094 },
		"vlan 1":          func(c *Config) { c.Provider.Proxmox.VLAN = 1 },
		"vmid base 100":   func(c *Config) { c.Provider.Proxmox.VMIDBase = 100 },
		"vmid at bound":   func(c *Config) { c.Provider.Proxmox.VMIDBase = 0xFFFFFF - 101 },
		"http endpoint":   func(c *Config) { c.Provider.Proxmox.Endpoint = "http://pve.lan:8006/" },
		"masters only":    func(c *Config) { c.Workers.Count = 0; c.Provider.Proxmox.VMIDBase = 0xFFFFFF - 12 },
		"pool and ha set": func(c *Config) { c.Provider.Proxmox.Pool = "okd"; c.Provider.Proxmox.HA = true },
	}
	for name, mutate := range passing {
		t.Run("accepts "+name, func(t *testing.T) {
			cfg := DefaultConfig()
			mutate(cfg)
			assertValid(t, cfg)
		})
	}
}
