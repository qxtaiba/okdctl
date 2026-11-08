package config

import "testing"

func TestValidate_Required(t *testing.T) {
	if result := DefaultConfig().Validate(); !result.IsValid() {
		t.Fatalf("DefaultConfig: %v", result)
	}
	var nilCfg *Config
	if nilCfg.Validate().IsValid() {
		t.Error("nil config accepted")
	}

	cases := []struct {
		field string
		blank func(*Config)
	}{
		{FieldClusterName, func(c *Config) { c.Cluster.Name = "" }},
		{FieldClusterDomain, func(c *Config) { c.Cluster.Domain = "" }},
		{FieldDistributionType, func(c *Config) { c.Distribution.Type = "" }},
		{FieldDistributionVersion, func(c *Config) { c.Distribution.Version = "" }},
		{FieldProviderType, func(c *Config) { c.Provider.Type = "" }},
		{FieldProxmoxEndpoint, func(c *Config) { c.Provider.Proxmox.Endpoint = "" }},
		{FieldProxmoxNodes, func(c *Config) { c.Provider.Proxmox.Nodes = nil }},
		{FieldProxmoxStorage, func(c *Config) { c.Provider.Proxmox.Storage = "" }},
		{FieldNetworkingMachineCIDR, func(c *Config) { c.Networking.MachineCIDR = "" }},
		{FieldNetworkingGateway, func(c *Config) { c.Networking.Gateway = "" }},
		{FieldNetworkingDNS, func(c *Config) { c.Networking.DNS = nil }},
		{FieldNetworkingAPIVIP, func(c *Config) { c.Networking.APIVIP = "" }},
		{FieldNetworkingIngressVIP, func(c *Config) { c.Networking.IngressVIP = "" }},
		{FieldNetworkingNodeIPStart, func(c *Config) { c.Networking.NodeIPStart = "" }},
		{FieldNetworkingPodCIDR, func(c *Config) { c.Networking.PodCIDR = "" }},
		{FieldNetworkingServiceCIDR, func(c *Config) { c.Networking.ServiceCIDR = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.blank(cfg)
			result := cfg.Validate()
			if len(result.Errors) != 1 || result.Errors[0].Field != tc.field {
				t.Errorf("Validate = %v; want exactly one error on %s", result, tc.field)
			}
		})
	}
}
