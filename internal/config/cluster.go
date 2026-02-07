// Package config provides configuration management for the CLI.
package config

// ═══════════════════════════════════════════════════════════════════════════════
// ROOT CONFIG
// ═══════════════════════════════════════════════════════════════════════════════

// Config represents the complete cluster configuration.
type Config struct {
	Cluster      ClusterConfig            `yaml:"cluster" json:"cluster" mapstructure:"cluster"`
	Distribution DistributionConfig       `yaml:"distribution" json:"distribution" mapstructure:"distribution"`
	Provider     ProviderConfig           `yaml:"provider" json:"provider" mapstructure:"provider"`
	Topology     TopologyConfig           `yaml:"topology" json:"topology" mapstructure:"topology"`
	Networking   NetworkingConfig         `yaml:"networking" json:"networking" mapstructure:"networking"`
	Addons       map[string]AddonConfig   `yaml:"addons,omitempty" json:"addons,omitempty" mapstructure:"addons"`
	Files        FilesConfig              `yaml:"files" json:"files" mapstructure:"files"`
	HTTPServer   HTTPServerConfig         `yaml:"http_server" json:"httpServer" mapstructure:"http_server"`
	Deployment   DeploymentConfig         `yaml:"deployment" json:"deployment" mapstructure:"deployment"`
	Disks        DisksConfig              `yaml:"disks,omitempty" json:"disks,omitempty" mapstructure:"disks"`
}

type ClusterConfig struct {
	Name   string `yaml:"name" json:"name" mapstructure:"name"`
	Domain string `yaml:"domain" json:"domain" mapstructure:"domain"`
}

type DistributionConfig struct {
	Type    DistributionType `yaml:"type" json:"type" mapstructure:"type"`
	Version string           `yaml:"version" json:"version" mapstructure:"version"`
}

// ═══════════════════════════════════════════════════════════════════════════════
// TOPOLOGY
// ═══════════════════════════════════════════════════════════════════════════════

type TopologyConfig struct {
	ControlPlane NodeConfig `yaml:"control_plane" json:"controlPlane" mapstructure:"control_plane"`
	Workers      NodeConfig `yaml:"workers" json:"workers" mapstructure:"workers"`
	Bootstrap    NodeConfig `yaml:"bootstrap,omitempty" json:"bootstrap,omitempty" mapstructure:"bootstrap"`
	VMIDBase     int        `yaml:"vm_id_base,omitempty" json:"vmIdBase,omitempty" mapstructure:"vm_id_base"` // Starting VM ID
}

type NodeConfig struct {
	Count  int `yaml:"count" json:"count" mapstructure:"count"`
	CPU    int `yaml:"cpu" json:"cpu" mapstructure:"cpu"`
	Memory int `yaml:"memory" json:"memory" mapstructure:"memory"` // in MB
	Disk   int `yaml:"disk" json:"disk" mapstructure:"disk"`       // in GB
}

// ═══════════════════════════════════════════════════════════════════════════════
// NETWORKING
// ═══════════════════════════════════════════════════════════════════════════════

type NetworkingConfig struct {
	MachineCIDR string   `yaml:"machine_cidr" json:"machineCidr" mapstructure:"machine_cidr"`
	PodCIDR     string   `yaml:"pod_cidr" json:"podCidr" mapstructure:"pod_cidr"`
	ServiceCIDR string   `yaml:"service_cidr" json:"serviceCidr" mapstructure:"service_cidr"`
	HostPrefix  int      `yaml:"host_prefix,omitempty" json:"hostPrefix,omitempty" mapstructure:"host_prefix"`
	Gateway     string   `yaml:"gateway" json:"gateway" mapstructure:"gateway"`
	DNS         []string `yaml:"dns" json:"dns" mapstructure:"dns"`

	StaticIP StaticIPConfig `yaml:"static_ip,omitempty" json:"staticIp,omitempty" mapstructure:"static_ip"`
	Bastion  BastionConfig  `yaml:"bastion,omitempty" json:"bastion,omitempty" mapstructure:"bastion"`
	MetalLB  MetalLBConfig  `yaml:"metallb,omitempty" json:"metallb,omitempty" mapstructure:"metallb"`
}

type StaticIPConfig struct {
	Start     string `yaml:"start" json:"start" mapstructure:"start"`
	Netmask   string `yaml:"netmask" json:"netmask" mapstructure:"netmask"`
	Interface string `yaml:"interface" json:"interface" mapstructure:"interface"`
	Gateway   string `yaml:"gateway" json:"gateway" mapstructure:"gateway"`
	DNS       string `yaml:"dns" json:"dns" mapstructure:"dns"`
}

// BastionConfig defines the bastion host (runs HAProxy for API load balancing).
type BastionConfig struct {
	IP string `yaml:"ip" json:"ip" mapstructure:"ip"`
}

type MetalLBConfig struct {
	Pool string `yaml:"pool" json:"pool" mapstructure:"pool"` // e.g., "192.168.1.205-192.168.1.230"
}

// ═══════════════════════════════════════════════════════════════════════════════
// ADDONS
// ═══════════════════════════════════════════════════════════════════════════════

// AddonConfig is the generic configuration for any addon, keyed by name.
type AddonConfig struct {
	Enabled  bool              `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	Settings map[string]string `yaml:"settings,omitempty" json:"settings,omitempty" mapstructure:"settings"`
}

// ═══════════════════════════════════════════════════════════════════════════════
// PROVIDER
// ═══════════════════════════════════════════════════════════════════════════════

type ProviderConfig struct {
	Type    ProviderType   `yaml:"type" json:"type" mapstructure:"type"`
	Proxmox *ProxmoxConfig `yaml:"proxmox,omitempty" json:"proxmox,omitempty" mapstructure:"proxmox"`
}

type ProxmoxConfig struct {
	Host        string `yaml:"host" json:"host" mapstructure:"host"`
	Node        string `yaml:"node" json:"node" mapstructure:"node"`
	Storage     string `yaml:"storage" json:"storage" mapstructure:"storage"`
	DataStorage string `yaml:"data_storage,omitempty" json:"dataStorage,omitempty" mapstructure:"data_storage"`
	ISOStorage  string `yaml:"iso_storage,omitempty" json:"isoStorage,omitempty" mapstructure:"iso_storage"`
	Bridge      string `yaml:"bridge,omitempty" json:"bridge,omitempty" mapstructure:"bridge"`
	FCOSIso     string `yaml:"fcos_iso,omitempty" json:"fcosIso,omitempty" mapstructure:"fcos_iso"`

	// Credentials — prefer openshitctl.env or environment variables
	Username string `yaml:"username,omitempty" json:"username,omitempty" mapstructure:"username"`
	Password string `yaml:"-" json:"-" mapstructure:"password"`
	APIToken string `yaml:"-" json:"-" mapstructure:"api_token"`
	TokenID  string `yaml:"token_id,omitempty" json:"tokenId,omitempty" mapstructure:"token_id"`
	Insecure bool   `yaml:"insecure,omitempty" json:"insecure,omitempty" mapstructure:"insecure"`
}

// ═══════════════════════════════════════════════════════════════════════════════
// DEPLOYMENT
// ═══════════════════════════════════════════════════════════════════════════════

type FilesConfig struct {
	PullSecret   string `yaml:"pull_secret" json:"pullSecret" mapstructure:"pull_secret"`
	SSHPublicKey string `yaml:"ssh_public_key" json:"sshPublicKey" mapstructure:"ssh_public_key"`
}

type HTTPServerConfig struct {
	Port             int    `yaml:"port" json:"port" mapstructure:"port"`
	Root             string `yaml:"root" json:"root" mapstructure:"root"`
	IgnitionServerIP string `yaml:"ignition_server_ip" json:"ignitionServerIp" mapstructure:"ignition_server_ip"`
}

type DeploymentConfig struct {
	TerraformEnv     string `yaml:"terraform_env,omitempty" json:"terraformEnv,omitempty" mapstructure:"terraform_env"`
	AutoApprove      bool   `yaml:"auto_approve,omitempty" json:"autoApprove,omitempty" mapstructure:"auto_approve"`
	Debug            bool   `yaml:"debug,omitempty" json:"debug,omitempty" mapstructure:"debug"`
	SkipDepsCheck    bool   `yaml:"skip_deps_check,omitempty" json:"skipDepsCheck,omitempty" mapstructure:"skip_deps_check"`
	BootstrapTimeout int    `yaml:"bootstrap_timeout,omitempty" json:"bootstrapTimeout,omitempty" mapstructure:"bootstrap_timeout"`
	InstallTimeout   int    `yaml:"install_timeout,omitempty" json:"installTimeout,omitempty" mapstructure:"install_timeout"`
}

type DisksConfig struct {
	OSSizeGB   int `yaml:"os_size_gb" json:"osSizeGb" mapstructure:"os_size_gb"`
	DataSizeGB int `yaml:"data_size_gb" json:"dataSizeGb" mapstructure:"data_size_gb"`
}
