package config

// DefaultOKDVersion is the release a freshly generated config pins.
const DefaultOKDVersion = "4.22.0-okd-scos.10"

// MinOKDVersion is the oldest release the agent-based install path supports.
const MinOKDVersion = "4.22.0-okd-scos.9"

// DefaultVMIDBase is the VMID of the first cluster VM.
const DefaultVMIDBase = 6000

// Field* constants name dotted Config paths used to tag ValidationError.Field.
const (
	FieldClusterName   = "cluster.name"
	FieldClusterDomain = "cluster.domain"

	FieldDistributionType    = "distribution.type"
	FieldDistributionVersion = "distribution.version"

	FieldProviderType      = "provider.type"
	FieldProxmoxEndpoint   = "provider.proxmox.endpoint"
	FieldProxmoxNodes      = "provider.proxmox.nodes"
	FieldProxmoxStorage    = "provider.proxmox.storage"
	FieldProxmoxISOStorage = "provider.proxmox.iso_storage"
	FieldProxmoxBridge     = "provider.proxmox.bridge"
	FieldProxmoxVLAN       = "provider.proxmox.vlan"
	FieldProxmoxCPUType    = "provider.proxmox.cpu_type"
	FieldProxmoxVMIDBase   = "provider.proxmox.vmid_base"

	FieldNetworkingMachineCIDR = "networking.machine_cidr"
	FieldNetworkingGateway     = "networking.gateway"
	FieldNetworkingDNS         = "networking.dns"
	FieldNetworkingAPIVIP      = "networking.api_vip"
	FieldNetworkingIngressVIP  = "networking.ingress_vip"
	FieldNetworkingNodeIPStart = "networking.node_ip_start"
	FieldNetworkingPodCIDR     = "networking.pod_cidr"
	FieldNetworkingServiceCIDR = "networking.service_cidr"
	FieldNetworkingHostPrefix  = "networking.host_prefix"

	FieldControlPlane = "control_plane"
	FieldWorkers      = "workers"
	FieldNodes        = "nodes"

	FieldFilesPullSecret   = "files.pull_secret" //nolint:gosec // G101: a config path, not a credential
	FieldFilesSSHPublicKey = "files.ssh_public_key"
)
