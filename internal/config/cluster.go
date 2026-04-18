// Package config defines the YAML-serializable deployment schema for
// okdctl (cluster, distribution, provider, topology, networking, addons)
// along with loaders, defaults, generators, and validators.
//
// Serialization uses sigs.k8s.io/yaml, which reads json struct tags as
// YAML key names. Only json tags are authoritative here; there are no
// separate yaml tags to maintain.
package config

type Config struct {
	Cluster      ClusterConfig          `json:"cluster"`
	Distribution DistributionConfig     `json:"distribution"`
	Provider     ProviderConfig         `json:"provider"`
	Topology     TopologyConfig         `json:"topology"`
	Networking   NetworkingConfig       `json:"networking"`
	Addons       map[string]AddonConfig `json:"addons,omitempty"`
	Files        FilesConfig            `json:"files"`
	HTTPServer   HTTPServerConfig       `json:"http_server"`
	Deployment   DeploymentConfig       `json:"deployment"`
	Disks        DisksConfig            `json:"disks,omitempty"`
}

type ClusterConfig struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

type DistributionConfig struct {
	Type    DistributionType `json:"type"`
	Version string           `json:"version"`
}

type TopologyConfig struct {
	ControlPlane NodeConfig `json:"control_plane"`
	Workers      NodeConfig `json:"workers"`
	Bootstrap    NodeConfig `json:"bootstrap,omitempty"`
	VMIDBase     int        `json:"vm_id_base,omitempty"`
}

type NodeConfig struct {
	Count  int `json:"count"`
	CPU    int `json:"cpu"`
	Memory int `json:"memory"` // in MB
	Disk   int `json:"disk"`   // in GB
}

type NetworkingConfig struct {
	MachineCIDR string   `json:"machine_cidr"`
	PodCIDR     string   `json:"pod_cidr"`
	ServiceCIDR string   `json:"service_cidr"`
	HostPrefix  int      `json:"host_prefix,omitempty"`
	Gateway     string   `json:"gateway"`
	DNS         []string `json:"dns"`

	StaticIP StaticIPConfig `json:"static_ip,omitempty"`
	Bastion  BastionConfig  `json:"bastion,omitempty"`
}

type StaticIPConfig struct {
	Start     string `json:"start"`
	Netmask   string `json:"netmask"`
	Interface string `json:"interface"`
	DNS       string `json:"dns"`
}

// BastionConfig defines the bastion host (runs HAProxy for API load balancing).
type BastionConfig struct {
	IP  string `json:"ip"`
	VIP string `json:"vip,omitempty"`
}

type AddonConfig struct {
	Enabled  bool              `json:"enabled"`
	Settings map[string]string `json:"settings,omitempty"`
}

type AdditionalNetwork struct {
	Bridge  string `json:"bridge"`
	Model   string `json:"model,omitempty"`
	VLANTag int    `json:"vlan_tag,omitempty"`
}

type ProviderConfig struct {
	Type    ProviderType   `json:"type"`
	Proxmox *ProxmoxConfig `json:"proxmox,omitempty"`
}

type ProxmoxConfig struct {
	Host        string `json:"host"`
	Node        string `json:"node"`
	Storage     string `json:"storage"`
	DataStorage string `json:"data_storage,omitempty"`
	ISOStorage  string `json:"iso_storage,omitempty"`
	Bridge      string `json:"bridge,omitempty"`
	FCOSIso     string `json:"fcos_iso,omitempty"`

	// Credentials are injected from okdctl.env / environment variables via
	// internal/credentials, never persisted in the YAML config. All three
	// fields carry `json:"-"` so sigs.k8s.io/yaml excludes them from both
	// load and save.
	Username string `json:"-"`
	Password string `json:"-"`
	APIToken string `json:"-"`

	TokenID            string              `json:"token_id,omitempty"`
	Insecure           bool                `json:"insecure,omitempty"`
	CPUType            string              `json:"cpu_type,omitempty"`
	AdditionalNetworks []AdditionalNetwork `json:"additional_networks,omitempty"`
	NUMAEnabled        bool                `json:"numa_enabled,omitempty"`
	MasterNodes        []string            `json:"master_nodes,omitempty"`
	WorkerNodes        []string            `json:"worker_nodes,omitempty"`
}

type FilesConfig struct {
	PullSecret   string `json:"pull_secret"`
	SSHPublicKey string `json:"ssh_public_key"`
}

type HTTPServerConfig struct {
	Port             int    `json:"port"`
	Root             string `json:"root"`
	IgnitionServerIP string `json:"ignition_server_ip"`
}

type DeploymentConfig struct {
	TerraformEnv     string `json:"terraform_env,omitempty"`
	AutoApprove      bool   `json:"auto_approve,omitempty"`
	Debug            bool   `json:"debug,omitempty"`
	SkipDepsCheck    bool   `json:"skip_deps_check,omitempty"`
	BootstrapTimeout int    `json:"bootstrap_timeout,omitempty"`
	InstallTimeout   int    `json:"install_timeout,omitempty"`
}

type DisksConfig struct {
	WorkerDataSizeGB int `json:"worker_data_size_gb"`
	MasterDataSizeGB int `json:"master_data_size_gb"`
}
