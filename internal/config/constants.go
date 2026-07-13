package config

// Field* constants name dotted paths into Config. They are used by the
// validator to tag each ValidationError with the offending field so the CLI
// can surface precise "cluster.domain: must not be empty" messages.
const (
	FieldClusterName   = "cluster.name"
	FieldClusterDomain = "cluster.domain"

	FieldDistributionType    = "distribution.type"
	FieldDistributionVersion = "distribution.version"

	FieldProviderType    = "provider.type"
	FieldProviderProxmox = "provider.proxmox"

	FieldTopologyControlPlaneCount  = "topology.control_plane.count"
	FieldTopologyControlPlaneCPU    = "topology.control_plane.cpu"
	FieldTopologyControlPlaneMemory = "topology.control_plane.memory_mb"
	FieldTopologyControlPlaneDisk   = "topology.control_plane.disk_gb"
	FieldTopologyWorkersCount       = "topology.workers.count"
	FieldTopologyWorkersCPU         = "topology.workers.cpu"
	FieldTopologyWorkersMemory      = "topology.workers.memory_mb"
	FieldTopologyWorkersDisk        = "topology.workers.disk_gb"

	FieldNetworkingMachineCIDR     = "networking.machine_cidr"
	FieldNetworkingPodCIDR         = "networking.pod_cidr"
	FieldNetworkingServiceCIDR     = "networking.service_cidr"
	FieldNetworkingGateway         = "networking.gateway"
	FieldNetworkingDNS             = "networking.dns"
	FieldNetworkingHostPrefix      = "networking.host_prefix"
	FieldNetworkingBastionIP       = "networking.bastion.ip"
	FieldNetworkingStaticIPStart   = "networking.static_ip.start"
	FieldNetworkingStaticIPNetmask = "networking.static_ip.netmask"
	FieldNetworkingStaticIPIface   = "networking.static_ip.interface"
	FieldNetworkingStaticIPDNS     = "networking.static_ip.dns"
	FieldNetworkingNTPServer       = "networking.ntp_server"

	FieldProxmoxHost                     = "provider.proxmox.host"
	FieldProxmoxNode                     = "provider.proxmox.node"
	FieldProxmoxStorage                  = "provider.proxmox.storage"
	FieldProxmoxISOStorage               = "provider.proxmox.iso_storage"
	FieldProxmoxDataStorage              = "provider.proxmox.data_storage"
	FieldProxmoxBridge                   = "provider.proxmox.bridge"
	FieldProxmoxCPUType                  = "provider.proxmox.cpu_type"
	FieldProxmoxInsecureHTTP             = "provider.proxmox.insecure_http"
	FieldProxmoxRequirePinnedFingerprint = "provider.proxmox.require_pinned_fingerprint"
	FieldProxmoxControlPlaneNodes        = "provider.proxmox.control_plane_nodes"
	FieldProxmoxWorkerNodes              = "provider.proxmox.worker_nodes"

	FieldFilesSSHPublicKey = "files.ssh_public_key"
	FieldFilesPullSecret   = "files.pull_secret" //nolint:gosec // field name constant, not a credential

	FieldHTTPServerIP   = "http_server.ip"
	FieldHTTPServerPort = "http_server.port"
	FieldHTTPServerRoot = "http_server.root"

	FieldDeploymentAutoApprove  = "deployment.auto_approve"
	FieldDeploymentTerraformEnv = "deployment.terraform_env"
	FieldDeploymentBinDir       = "deployment.bin_dir"
)
