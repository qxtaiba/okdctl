package config

import "slices"

const (
	FieldClusterName   = "cluster.name"
	FieldClusterDomain = "cluster.domain"

	FieldDistributionType    = "distribution.type"
	FieldDistributionVersion = "distribution.version"

	FieldProviderType    = "provider.type"
	FieldProviderProxmox = "provider.proxmox"

	FieldTopologyControlPlaneCount  = "topology.control_plane.count"
	FieldTopologyControlPlaneCPU    = "topology.control_plane.cpu"
	FieldTopologyControlPlaneMemory = "topology.control_plane.memory"
	FieldTopologyControlPlaneDisk   = "topology.control_plane.disk"
	FieldTopologyWorkersCount       = "topology.workers.count"
	FieldTopologyWorkersCPU         = "topology.workers.cpu"
	FieldTopologyWorkersMemory      = "topology.workers.memory"
	FieldTopologyWorkersDisk        = "topology.workers.disk"

	FieldNetworkingMachineCIDR     = "networking.machine_cidr"
	FieldNetworkingPodCIDR         = "networking.pod_cidr"
	FieldNetworkingServiceCIDR     = "networking.service_cidr"
	FieldNetworkingGateway         = "networking.gateway"
	FieldNetworkingDNS             = "networking.dns"
	FieldNetworkingHostPrefix      = "networking.host_prefix"
	FieldNetworkingBastionIP       = "networking.bastion.ip"
	FieldNetworkingStaticIPStart   = "networking.static_ip.start"
	FieldNetworkingStaticIPNetmask = "networking.static_ip.netmask"
	FieldNetworkingStaticIPIface = "networking.static_ip.interface"

	FieldProxmoxHost    = "provider.proxmox.host"
	FieldProxmoxNode    = "provider.proxmox.node"
	FieldProxmoxStorage = "provider.proxmox.storage"

	FieldFilesSSHPublicKey = "files.ssh_public_key"
	FieldFilesPullSecret   = "files.pull_secret"

	FieldHTTPServerIP   = "http_server.ip"
	FieldHTTPServerPort = "http_server.port"

	FieldDeploymentAutoApprove  = "deployment.auto_approve"
	FieldDeploymentTerraformEnv = "deployment.terraform_env"
)

func IsValidEnum(value string, validValues []string) bool {
	return slices.Contains(validValues, value)
}
