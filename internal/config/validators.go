package config

func validateRequired(cfg *Config, result *ValidationResult) {
	checks := []struct {
		bad   bool
		field string
	}{
		{cfg.Cluster.Name == "", FieldClusterName},
		{cfg.Cluster.Domain == "", FieldClusterDomain},
		{cfg.Distribution.Type == "", FieldDistributionType},
		{cfg.Distribution.Version == "", FieldDistributionVersion},
		{cfg.Provider.Type == "", FieldProviderType},
		{cfg.Provider.Proxmox.Endpoint == "", FieldProxmoxEndpoint},
		{len(cfg.Provider.Proxmox.Nodes) == 0, FieldProxmoxNodes},
		{cfg.Provider.Proxmox.Storage == "", FieldProxmoxStorage},
		{cfg.Networking.MachineCIDR == "", FieldNetworkingMachineCIDR},
		{cfg.Networking.Gateway == "", FieldNetworkingGateway},
		{len(cfg.Networking.DNS) == 0, FieldNetworkingDNS},
		{cfg.Networking.APIVIP == "", FieldNetworkingAPIVIP},
		{cfg.Networking.IngressVIP == "", FieldNetworkingIngressVIP},
		{cfg.Networking.NodeIPStart == "", FieldNetworkingNodeIPStart},
		{cfg.Networking.PodCIDR == "", FieldNetworkingPodCIDR},
		{cfg.Networking.ServiceCIDR == "", FieldNetworkingServiceCIDR},
	}
	for _, c := range checks {
		if c.bad {
			result.AddError(c.field, "required; set it in the config file")
		}
	}
}
