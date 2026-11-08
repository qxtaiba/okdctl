package config

import "testing"

func TestValidateNodes(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   finding
	}{
		{"master cpu floor", func(c *Config) { c.ControlPlane.CPU = 3 }, finding{FieldControlPlane + ".cpu", "at least 4"}},
		{"master memory floor", func(c *Config) { c.ControlPlane.MemoryMiB = 16383 }, finding{FieldControlPlane + ".memory_mib", "at least 16384"}},
		{"master root disk floor", func(c *Config) { c.ControlPlane.Disks[0].SizeGiB = 99 }, finding{FieldControlPlane + ".disks[0].size_gib", "at least 100"}},
		{"master no disks", func(c *Config) { c.ControlPlane.Disks = nil }, finding{FieldControlPlane + ".disks", "at least one disk"}},
		{"worker cpu floor", func(c *Config) { c.Workers.CPU = 1 }, finding{FieldWorkers + ".cpu", "at least 2"}},
		{"worker memory floor", func(c *Config) { c.Workers.MemoryMiB = 8191 }, finding{FieldWorkers + ".memory_mib", "at least 8192"}},
		{"worker root disk floor", func(c *Config) { c.Workers.Disks[0].SizeGiB = 50 }, finding{FieldWorkers + ".disks[0].size_gib", "at least 100"}},
		{"worker data disk zero", func(c *Config) { c.Workers.Disks[1].SizeGiB = 0 }, finding{FieldWorkers + ".disks[1].size_gib", "greater than 0"}},
		{"worker count negative", func(c *Config) { c.Workers.Count = -5 }, finding{FieldWorkers + ".count", "0 or more"}},
		{
			"unknown key with hyphen",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-worker-3": {Host: "pve"}} },
			finding{"nodes.lab-worker-3", "not a node name this config produces; keys match lab-master<0..2> or lab-worker<i>"},
		},
		{
			"unknown key without cluster prefix",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"worker0": {Host: "pve"}} },
			finding{"nodes.worker0", "not a node name this config produces; keys match lab-master<0..2> or lab-worker<i>"},
		},
		{
			"master index out of range",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-master3": {Host: "pve"}} },
			finding{"nodes.lab-master3", "not a node name this config produces; keys match lab-master<0..2> or lab-worker<i>"},
		},
		{
			"more worker overrides than count",
			func(c *Config) {
				c.Nodes = map[string]NodeOverride{"lab-worker0": {Host: "pve"}, "lab-worker1": {Host: "pve"}, "lab-worker2": {Host: "pve"}}
			},
			finding{FieldNodes, "3 worker overrides exceed workers.count 2"},
		},
		{
			"override cpu floor",
			func(c *Config) { cpu := 2; c.Nodes = map[string]NodeOverride{"lab-master1": {CPU: &cpu}} },
			finding{"nodes.lab-master1.cpu", "at least 4"},
		},
		{
			"override memory floor",
			func(c *Config) { mem := 4096; c.Nodes = map[string]NodeOverride{"lab-worker0": {MemoryMiB: &mem}} },
			finding{"nodes.lab-worker0.memory_mib", "at least 8192"},
		},
		{
			"override root disk floor",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-worker0": {Disks: []DiskSpec{{SizeGiB: 20}}}} },
			finding{"nodes.lab-worker0.disks[0].size_gib", "at least 100"},
		},
		{
			"override data disk zero",
			func(c *Config) {
				c.Nodes = map[string]NodeOverride{"lab-master2": {Disks: []DiskSpec{{SizeGiB: 120}, {SizeGiB: -1}}}}
			},
			finding{"nodes.lab-master2.disks[1].size_gib", "greater than 0"},
		},
		{
			"override ip parse",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-worker0": {IP: "192.168.1"}} },
			finding{"nodes.lab-worker0.ip", "not an IP address"},
		},
		{
			"override ip outside",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-worker0": {IP: "192.168.9.9"}} },
			finding{"nodes.lab-worker0.ip", "outside machine_cidr 192.168.1.0/24"},
		},
		{
			"two overrides share an ip",
			func(c *Config) {
				c.Nodes = map[string]NodeOverride{"lab-worker0": {IP: "192.168.1.170"}, "lab-worker1": {IP: "192.168.1.170"}}
			},
			finding{"nodes.lab-worker1.ip", "lab-worker1 and lab-worker0 share 192.168.1.170"},
		},
		{
			"override ip is a derived address",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-worker1": {IP: "192.168.1.152"}} },
			finding{"nodes.lab-worker1.ip", "lab-worker1 and lab-master2 share 192.168.1.152"},
		},
		{
			"override ip is api vip",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-worker0": {IP: "192.168.1.158"}} },
			finding{"nodes.lab-worker0.ip", "lab-worker0 and api_vip share 192.168.1.158"},
		},
		{
			"override ip is ingress vip",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-master0": {IP: "192.168.1.159"}} },
			finding{"nodes.lab-master0.ip", "lab-master0 and ingress_vip share 192.168.1.159"},
		},
		{
			"override ip is gateway",
			func(c *Config) { c.Nodes = map[string]NodeOverride{"lab-worker0": {IP: "192.168.1.1"}} },
			finding{"nodes.lab-worker0.ip", "lab-worker0 and gateway share 192.168.1.1"},
		},
		{
			"override ip is resolver",
			func(c *Config) {
				c.Networking.DNS = []string{"192.168.1.1", "192.168.1.53"}
				c.Nodes = map[string]NodeOverride{"lab-worker0": {IP: "192.168.1.53"}}
			},
			finding{"nodes.lab-worker0.ip", "lab-worker0 and dns[1] share 192.168.1.53"},
		},
		{
			"override host outside the pool",
			func(c *Config) {
				c.Provider.Proxmox.Nodes = []string{"pve1", "pve2"}
				c.Nodes = map[string]NodeOverride{"lab-worker0": {Host: "pve9"}}
			},
			finding{"nodes.lab-worker0.host", `lab-worker0 host "pve9" is not one of provider.proxmox.nodes (pve1, pve2)`},
		},
		{
			"disk serial too long",
			func(c *Config) { c.Provider.Proxmox.VMIDBase = 1_000_000_000_000_000_000 },
			finding{"nodes.lab-master0.disks[0]", "serial okd1000000000000000010d0 exceeds 20 characters"},
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
		"worker hole": func(c *Config) {
			c.Nodes = map[string]NodeOverride{"lab-worker0": {Host: "pve"}, "lab-worker2": {Host: "pve"}}
		},
		"override at floors": func(c *Config) {
			cpu, mem := 4, 16384
			c.Nodes = map[string]NodeOverride{"lab-master0": {CPU: &cpu, MemoryMiB: &mem, Disks: []DiskSpec{{SizeGiB: 100}}}}
		},
		"distinct override ips": func(c *Config) {
			c.Nodes = map[string]NodeOverride{"lab-worker0": {IP: "192.168.1.170"}, "lab-worker1": {IP: "192.168.1.171"}}
		},
		"no workers block": func(c *Config) { c.Workers = WorkersConfig{} },
		"override host in the pool": func(c *Config) {
			c.Provider.Proxmox.Nodes = []string{"pve1", "pve2"}
			c.Nodes = map[string]NodeOverride{"lab-master1": {Host: "pve2"}}
		},
	}
	for name, mutate := range passing {
		t.Run("accepts "+name, func(t *testing.T) {
			cfg := DefaultConfig()
			mutate(cfg)
			assertValid(t, cfg)
		})
	}
}
