// Package config defines the YAML-serializable deployment schema for
// okdctl (cluster, distribution, provider, topology, networking, addons)
// along with loaders, defaults, generators, and validators.
//
// Serialization uses sigs.k8s.io/yaml, which reads json struct tags as
// YAML key names. Only json tags are authoritative here; there are no
// separate yaml tags to maintain.
package config

// SchemaVersionV1 is the current okdctl.yaml schema marker. Loader rejects
// configs that do not match — bump this value (and add a migration) only
// when the schema makes a breaking change.
const SchemaVersionV1 = "v1"

// Config is the root okdctl.yaml schema.
type Config struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Cluster       ClusterConfig          `json:"cluster"`
	Distribution  DistributionConfig     `json:"distribution"`
	Provider      ProviderConfig         `json:"provider"`
	Topology      TopologyConfig         `json:"topology"`
	Networking    NetworkingConfig       `json:"networking"`
	Addons        map[string]AddonConfig `json:"addons,omitempty"`
	Files         FilesConfig            `json:"files"`
	HTTPServer    HTTPServerConfig       `json:"http_server"`
	Deployment    DeploymentConfig       `json:"deployment"`
	Disks         DisksConfig            `json:"disks,omitempty"`
}

// ClusterConfig configures the cluster's identity (name and base domain).
type ClusterConfig struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// DistributionConfig selects the Kubernetes distribution and version.
type DistributionConfig struct {
	Type    DistributionType `json:"type"`
	Version string           `json:"version"`
}

// TopologyConfig configures the control-plane, worker, and bootstrap node
// groups plus the VMID base used when numbering provisioned VMs.
type TopologyConfig struct {
	ControlPlane NodeConfig `json:"control_plane"`
	Workers      NodeConfig `json:"workers"`
	Bootstrap    NodeConfig `json:"bootstrap,omitempty"`
	VMIDBase     int        `json:"vm_id_base,omitempty"`
}

// NodeConfig configures the count and per-node resources for a node group.
type NodeConfig struct {
	Count  int `json:"count"`
	CPU    int `json:"cpu"`
	Memory int `json:"memory"` // in MB
	Disk   int `json:"disk"`   // in GB
}

// NetworkingConfig configures the cluster's machine, pod, and service CIDRs
// along with gateway, DNS, and optional static-IP / bastion settings.
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

// StaticIPConfig describes the starting IP, netmask, interface, and DNS
// used when assigning static addresses to cluster nodes.
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

// AddonConfig toggles an optional cluster feature and carries its settings.
type AddonConfig struct {
	Enabled  bool              `json:"enabled"`
	Settings map[string]string `json:"settings,omitempty"`
}

// AdditionalNetwork describes an extra NIC attached to each VM.
type AdditionalNetwork struct {
	Bridge  string `json:"bridge"`
	Model   string `json:"model,omitempty"`
	VLANTag int    `json:"vlan_tag,omitempty"`
}

// ProviderConfig selects the infrastructure provider and its settings.
type ProviderConfig struct {
	Type    ProviderType   `json:"type"`
	Proxmox *ProxmoxConfig `json:"proxmox,omitempty"`
}

// ProxmoxConfig configures the Proxmox VE provider. Credential fields carry
// json:"-" and are populated from env/config separately — never persisted
// to okdctl.yaml.
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

// FilesConfig points at the pull-secret and SSH public key files injected
// into ignition/cloud-init.
type FilesConfig struct {
	PullSecret   string `json:"pull_secret"`
	SSHPublicKey string `json:"ssh_public_key"`
}

// HTTPServerConfig configures the local HTTP server that hosts ignition
// payloads during install.
type HTTPServerConfig struct {
	Port             int    `json:"port"`
	Root             string `json:"root"`
	IgnitionServerIP string `json:"ignition_server_ip"`
}

// ToolVersionOverride lets operators pin a tool version or redirect its
// download URL. URLTemplate may contain {version} and {arch} placeholders
// substituted at install time; a URL without placeholders is used verbatim.
// Version overrides the default version used in URL construction, so
// setting only Version swaps the version embedded in the default template.
type ToolVersionOverride struct {
	Version     string `json:"version,omitempty"`
	URLTemplate string `json:"url_template,omitempty"`
}

// DeploymentConfig tunes deployment-time behavior: Terraform environment,
// auto-approve, timeouts, per-tool version overrides, and the directory where
// setup installs managed binaries.
type DeploymentConfig struct {
	TerraformEnv     string                         `json:"terraform_env,omitempty"`
	AutoApprove      bool                           `json:"auto_approve,omitempty"`
	Debug            bool                           `json:"debug,omitempty"`
	SkipDepsCheck    bool                           `json:"skip_deps_check,omitempty"`
	BootstrapTimeout int                            `json:"bootstrap_timeout,omitempty"`
	InstallTimeout   int                            `json:"install_timeout,omitempty"`
	BinDir           string                         `json:"bin_dir,omitempty"`
	ToolVersions     map[string]ToolVersionOverride `json:"tool_versions,omitempty"`
}

// DisksConfig sets optional extra data-disk sizes attached to master/worker
// nodes.
type DisksConfig struct {
	WorkerDataSizeGB int `json:"worker_data_size_gb"`
	MasterDataSizeGB int `json:"master_data_size_gb"`
}
