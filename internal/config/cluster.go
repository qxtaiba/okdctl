// Package config defines the YAML-serializable deployment schema for okdctl.
// Serialization uses sigs.k8s.io/yaml, mapping json struct tags to YAML keys.
package config

// Schema version markers for okdctl.yaml; bump only on a breaking change.
const (
	SchemaVersionV2 = "v2"

	SchemaVersionCurrent = SchemaVersionV2
)

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
	Disks         DisksConfig            `json:"disks,omitzero"`
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
	// Bootstrap sizes the single ephemeral pivot VM: Count must be 1 (or
	// omitted); CPU/MemoryMB default to control-plane; DiskGB always
	// matches the control-plane OS disk.
	Bootstrap NodeConfig `json:"bootstrap,omitzero"`
	VMIDBase  int        `json:"vm_id_base,omitempty"`
}

// NodeConfig configures the count and per-node resources for a node group.
// DiskGB sizes only the root/OS disk — extra data disks live in DisksConfig,
// and per-VM placement in ProxmoxConfig.ControlPlaneNodes/WorkerNodes.
type NodeConfig struct {
	Count    int `json:"count"`
	CPU      int `json:"cpu"`
	MemoryMB int `json:"memory_mb"`
	DiskGB   int `json:"disk_gb"`
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

	StaticIP StaticIPConfig `json:"static_ip,omitzero"`
	Bastion  BastionConfig  `json:"bastion,omitzero"`

	// NTPServer is the chrony source for master/worker MachineConfig; empty
	// uses HTTPServer.IgnitionServerIP.
	NTPServer string `json:"ntp_server,omitempty"`
}

// StaticIPConfig describes the static address plan for cluster nodes.
type StaticIPConfig struct {
	// Start is the bootstrap node's IP (not the first free address):
	// masters/workers allocate from Start+1, and the API VIP derives as
	// .10 unless bastion.vip overrides. It must not equal a live host or
	// the bootstrap VM ARP-fights it.
	Start string `json:"start"`
	// Netmask is derived from machine_cidr at load time — a YAML value is
	// overwritten; persisted only to carry the dotted form for kernel args
	// and HAProxy/dnsmasq templates.
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

// ProxmoxConfig configures the Proxmox VE provider.
type ProxmoxConfig struct {
	Host        string `json:"host"`
	Node        string `json:"node"`
	Storage     string `json:"storage"`
	DataStorage string `json:"data_storage,omitempty"`
	ISOStorage  string `json:"iso_storage,omitempty"`
	Bridge      string `json:"bridge,omitempty"`
	FCOSIso     string `json:"fcos_iso,omitempty"`

	// Credentials come from internal/credentials (okdctl.env/env vars),
	// never persisted; json:"-" excludes them from load and save.
	Username string      `json:"-"`
	Password SecretBytes `json:"-"`
	APIToken SecretBytes `json:"-"`

	TokenID  string `json:"token_id,omitempty"`
	Insecure bool   `json:"insecure,omitempty"`
	// InsecureHTTP allows http:// endpoints — basic-auth over plaintext, opt-in only.
	InsecureHTTP       bool                `json:"insecure_http,omitempty"`
	CPUType            string              `json:"cpu_type,omitempty"`
	AdditionalNetworks []AdditionalNetwork `json:"additional_networks,omitempty"`
	NUMAEnabled        bool                `json:"numa_enabled,omitempty"`
	// HAEnabled provisions Proxmox HA-manager anti-affinity for masters
	// (requires PVE 9+; see proxmox-okd/ha.tf).
	HAEnabled bool `json:"ha_enabled,omitempty"`
	// ControlPlaneNodes/WorkerNodes assign VMs by index to a Proxmox node
	// (short lists pad with Node, long ones fail validation); rendered into
	// terraform's master_target_nodes/worker_target_nodes vars.
	ControlPlaneNodes []string `json:"control_plane_nodes,omitempty"`
	WorkerNodes       []string `json:"worker_nodes,omitempty"`
	// SSHHostFingerprint pins the Proxmox host key (SHA256:<base64> from
	// ssh-keygen -lf); set means mismatches are refused, unset means
	// accept-new TOFU with the fingerprint logged at WARN.
	SSHHostFingerprint string `json:"ssh_host_fingerprint,omitempty"`
	// RequirePinnedFingerprint fails closed when SSHHostFingerprint is unset
	// (sshpin.Verify returns AuthError instead of WARN+TOFU); default false
	// preserves accept-new.
	RequirePinnedFingerprint bool `json:"require_pinned_fingerprint,omitempty"`
}

// redactedProxmoxConfig is ProxmoxConfig's Redacted() projection, omitting
// Username/Password/APIToken so slog.Any can't reach them via RedactHandler.
type redactedProxmoxConfig struct {
	Host                     string
	Node                     string
	Storage                  string
	DataStorage              string
	ISOStorage               string
	Bridge                   string
	FCOSIso                  string
	TokenID                  string
	Insecure                 bool
	InsecureHTTP             bool
	CPUType                  string
	AdditionalNetworks       []AdditionalNetwork
	NUMAEnabled              bool
	HAEnabled                bool
	ControlPlaneNodes        []string
	WorkerNodes              []string
	SSHHostFingerprint       string
	RequirePinnedFingerprint bool
}

// Redacted returns p's non-credential fields for logutil.redactAny.
func (p *ProxmoxConfig) Redacted() any {
	if p == nil {
		return nil
	}
	return redactedProxmoxConfig{
		Host:                     p.Host,
		Node:                     p.Node,
		Storage:                  p.Storage,
		DataStorage:              p.DataStorage,
		ISOStorage:               p.ISOStorage,
		Bridge:                   p.Bridge,
		FCOSIso:                  p.FCOSIso,
		TokenID:                  p.TokenID,
		Insecure:                 p.Insecure,
		InsecureHTTP:             p.InsecureHTTP,
		CPUType:                  p.CPUType,
		AdditionalNetworks:       p.AdditionalNetworks,
		NUMAEnabled:              p.NUMAEnabled,
		HAEnabled:                p.HAEnabled,
		ControlPlaneNodes:        p.ControlPlaneNodes,
		WorkerNodes:              p.WorkerNodes,
		SSHHostFingerprint:       p.SSHHostFingerprint,
		RequirePinnedFingerprint: p.RequirePinnedFingerprint,
	}
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
	Root             string `json:"root"`
	IgnitionServerIP string `json:"ignition_server_ip"`
}

// DeploymentConfig tunes deployment-time behavior: Terraform environment,
// auto-approve, timeouts, and the directory where setup installs managed
// binaries.
type DeploymentConfig struct {
	TerraformEnv     string `json:"terraform_env,omitempty"`
	AutoApprove      bool   `json:"auto_approve,omitempty"`
	BootstrapTimeout int    `json:"bootstrap_timeout,omitempty"`
	InstallTimeout   int    `json:"install_timeout,omitempty"`
	BinDir           string `json:"bin_dir,omitempty"`
}

// TerraformEnvName returns the active Terraform environment name from
// deployment.terraform_env, defaulting to "production" when unset.
func (c *Config) TerraformEnvName() string {
	if c.Deployment.TerraformEnv != "" {
		return c.Deployment.TerraformEnv
	}
	return defaultTerraformEnv
}

// DisksConfig sizes the optional extra disks per node group (data disk per
// role, ceph mon-store on control-plane); the OS disk is
// topology.<group>.disk_gb. A size of 0 omits the disk — zeroing one after
// apply makes terraform destroy it on the next run.
type DisksConfig struct {
	WorkerDataSizeGB       int `json:"worker_data_size_gb"`
	ControlPlaneDataSizeGB int `json:"control_plane_data_size_gb"`
	ControlPlaneMonSizeGB  int `json:"control_plane_mon_size_gb"`
}
