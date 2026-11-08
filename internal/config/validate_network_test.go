package config

import (
	"strings"
	"testing"
)

func TestValidateNetwork(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   finding
	}{
		{"machine cidr", func(c *Config) { c.Networking.MachineCIDR = "192.168.1.0/40" }, finding{FieldNetworkingMachineCIDR, "not a CIDR"}},
		{"pod cidr", func(c *Config) { c.Networking.PodCIDR = "10.128.0.0" }, finding{FieldNetworkingPodCIDR, "not a CIDR"}},
		{"service cidr", func(c *Config) { c.Networking.ServiceCIDR = "" }, finding{FieldNetworkingServiceCIDR, "not a CIDR"}},
		{"gateway", func(c *Config) { c.Networking.Gateway = "999.1.1.1" }, finding{FieldNetworkingGateway, "not an IP address"}},
		{"no dns", func(c *Config) { c.Networking.DNS = nil }, finding{FieldNetworkingDNS, "at least one"}},
		{"dns entry", func(c *Config) { c.Networking.DNS = []string{"1.1.1.1", "dns.lan"} }, finding{FieldNetworkingDNS + "[1]", "not an IP address"}},
		{"api vip parse", func(c *Config) { c.Networking.APIVIP = "x" }, finding{FieldNetworkingAPIVIP, "not an IP address"}},
		{"api vip outside", func(c *Config) { c.Networking.APIVIP = "192.168.2.158" }, finding{FieldNetworkingAPIVIP, "outside machine_cidr 192.168.1.0/24"}},
		{"ingress vip outside", func(c *Config) { c.Networking.IngressVIP = "10.0.0.1" }, finding{FieldNetworkingIngressVIP, "outside machine_cidr 192.168.1.0/24"}},
		{"vips equal", func(c *Config) { c.Networking.IngressVIP = c.Networking.APIVIP }, finding{FieldNetworkingIngressVIP, "equals api_vip"}},
		{"api vip is gateway", func(c *Config) { c.Networking.APIVIP = "192.168.1.1" }, finding{FieldNetworkingAPIVIP, "equals gateway"}},
		{"ingress vip is resolver", func(c *Config) { c.Networking.DNS = []string{"192.168.1.1", "192.168.1.159"} }, finding{FieldNetworkingIngressVIP, "equals dns[1]"}},
		{"api vip is master address", func(c *Config) { c.Networking.APIVIP = "192.168.1.151" }, finding{FieldNetworkingAPIVIP, "equals the address of lab-master1"}},
		{"ingress vip is worker address", func(c *Config) { c.Networking.IngressVIP = "192.168.1.161" }, finding{FieldNetworkingIngressVIP, "equals the address of lab-worker1"}},
		{"node ip start parse", func(c *Config) { c.Networking.NodeIPStart = "" }, finding{FieldNetworkingNodeIPStart, "not an IP address"}},
		{"node range starts outside", func(c *Config) { c.Networking.NodeIPStart = "192.168.2.10" }, finding{FieldNetworkingNodeIPStart, "outside machine_cidr 192.168.1.0/24"}},
		{"node range ends outside", func(c *Config) { c.Networking.NodeIPStart = "192.168.1.245" }, finding{FieldNetworkingNodeIPStart, "192.168.1.245-192.168.2.0 leaves machine_cidr 192.168.1.0/24"}},
		{"node start is gateway", func(c *Config) { c.Networking.NodeIPStart = "192.168.1.1" }, finding{FieldNetworkingNodeIPStart, "lab-master0 and gateway share 192.168.1.1"}},
		{"master is resolver", func(c *Config) { c.Networking.DNS = []string{"192.168.1.1", "192.168.1.151"} }, finding{FieldNetworkingNodeIPStart, "lab-master1 and dns[1] share 192.168.1.151"}},
		{"worker is resolver", func(c *Config) { c.Networking.DNS = []string{"192.168.1.161"} }, finding{FieldNetworkingNodeIPStart, "lab-worker1 and dns[0] share 192.168.1.161"}},
		{"node start is network address", func(c *Config) { c.Networking.NodeIPStart = "192.168.1.0" }, finding{FieldNetworkingNodeIPStart, "lab-master0 lands on 192.168.1.0, the network address of machine_cidr 192.168.1.0/24"}},
		{"worker lands on broadcast", func(c *Config) { c.Networking.NodeIPStart = "192.168.1.244" }, finding{FieldNetworkingNodeIPStart, "lab-worker1 lands on 192.168.1.255, the broadcast address of machine_cidr 192.168.1.0/24"}},
		{
			"node range ends outside via worker index",
			func(c *Config) {
				c.Networking.NodeIPStart = "192.168.1.200"
				c.Nodes = map[string]NodeOverride{"lab-worker100": {Host: "pve"}}
			},
			finding{FieldNetworkingNodeIPStart, "192.168.1.200-192.168.2.54 leaves machine_cidr 192.168.1.0/24"},
		},
		{"pod overlaps machine", func(c *Config) { c.Networking.PodCIDR = "192.168.0.0/16" }, finding{FieldNetworkingPodCIDR, "overlaps machine_cidr"}},
		{"service overlaps machine", func(c *Config) { c.Networking.ServiceCIDR = "192.168.1.0/25" }, finding{FieldNetworkingServiceCIDR, "overlaps machine_cidr"}},
		{"service overlaps pod", func(c *Config) { c.Networking.ServiceCIDR = "10.128.0.0/16" }, finding{FieldNetworkingServiceCIDR, "overlaps pod_cidr"}},
		{"host prefix low", func(c *Config) { c.Networking.HostPrefix = 19 }, finding{FieldNetworkingHostPrefix, "between 20 and 28"}},
		{"host prefix high", func(c *Config) { c.Networking.HostPrefix = 29 }, finding{FieldNetworkingHostPrefix, "between 20 and 28"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mutate(cfg)
			assertFinding(t, cfg, tc.want)
		})
	}

	passing := map[string]func(*Config){
		"gateway outside machine_cidr": func(c *Config) { c.Networking.Gateway = "10.0.0.1" },
		"public resolvers":             func(c *Config) { c.Networking.DNS = []string{"1.1.1.1", "9.9.9.9"} },
		"range ends on last host":      func(c *Config) { c.Networking.NodeIPStart = "192.168.1.243" },
		"host prefix 20":               func(c *Config) { c.Networking.HostPrefix = 20 },
		"host prefix 28":               func(c *Config) { c.Networking.HostPrefix = 28 },
		"masters only range":           func(c *Config) { c.Workers.Count = 0; c.Networking.NodeIPStart = "192.168.1.252" },
		"ntp listed":                   func(c *Config) { c.Networking.NTP = []string{"pool.ntp.org"} },
	}
	for name, mutate := range passing {
		t.Run("accepts "+name, func(t *testing.T) {
			cfg := DefaultConfig()
			mutate(cfg)
			assertValid(t, cfg)
		})
	}
}

func TestValidateNetwork_DerivedAddressRule(t *testing.T) {
	t.Run("skips a /31", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Networking.MachineCIDR = "192.168.1.0/31"
		cfg.Networking.NodeIPStart = "192.168.1.0"
		for _, e := range cfg.Validate().Errors {
			if strings.Contains(e.Message, "network address") || strings.Contains(e.Message, "broadcast address") {
				t.Errorf("edge rule fired on a point-to-point prefix: %v", e)
			}
		}
	})
	t.Run("skips an overridden node", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Nodes = map[string]NodeOverride{"lab-worker0": {IP: "192.168.1.1"}}
		assertFinding(t, cfg, finding{"nodes.lab-worker0.ip", "lab-worker0 and gateway share 192.168.1.1"})
		for _, e := range cfg.Validate().Errors {
			if e.Field == FieldNetworkingNodeIPStart {
				t.Errorf("derived rule reported an override: %v", e)
			}
		}
	})
}
