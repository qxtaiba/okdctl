package config

// DefaultOKDVersion is the release a freshly generated config pins.
const DefaultOKDVersion = "4.22.0-okd-scos.10"

// DefaultVMIDBase is the VMID of the first cluster VM.
const DefaultVMIDBase = 6000

// Field* constants name dotted Config paths used to tag ValidationError.Field.
const (
	FieldClusterName   = "cluster.name"
	FieldClusterDomain = "cluster.domain"

	FieldDistributionType    = "distribution.type"
	FieldDistributionVersion = "distribution.version"

	FieldProviderType    = "provider.type"
	FieldProxmoxEndpoint = "provider.proxmox.endpoint"
	FieldProxmoxNodes    = "provider.proxmox.nodes"
	FieldProxmoxStorage  = "provider.proxmox.storage"

	FieldNetworkingMachineCIDR = "networking.machine_cidr"
	FieldNetworkingGateway     = "networking.gateway"
	FieldNetworkingDNS         = "networking.dns"
	FieldNetworkingAPIVIP      = "networking.api_vip"
	FieldNetworkingIngressVIP  = "networking.ingress_vip"
	FieldNetworkingNodeIPStart = "networking.node_ip_start"
	FieldNetworkingPodCIDR     = "networking.pod_cidr"
	FieldNetworkingServiceCIDR = "networking.service_cidr"
)
