// Package config defines the YAML-serializable deployment schema for okdctl.
// Serialization uses sigs.k8s.io/yaml, which maps json struct tags to YAML key names.
// Only json tags are authoritative; maintain no separate yaml tags.
package config

// Schema version markers for okdctl.yaml. The loader accepts only
// SchemaVersionCurrent; bump it (and teach LoadFile a targeted migration
// message) only when the schema makes a breaking change.
const (
	// SchemaVersionV1 predates the v2 key renames (see CHANGELOG.md
	// "Unreleased"); kept so the loader can emit a migration diagnostic.
	SchemaVersionV1 = "v1"
	// SchemaVersionV2 renamed provider.proxmox.master_nodes →
	// control_plane_nodes, disks.master_data_size_gb →
	// control_plane_data_size_gb, and topology memory/disk →
	// memory_mb/disk_gb.
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
	Bootstrap    NodeConfig `json:"bootstrap,omitzero"`
	VMIDBase     int        `json:"vm_id_base,omitempty"`
}

// NodeConfig configures the count and per-node resources for a node group.
// DiskGB sizes the root/OS disk only; the optional extra data disk is sized
// per group in DisksConfig, and per-VM Proxmox placement lives in
// ProxmoxConfig.ControlPlaneNodes / WorkerNodes.
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

	// NTPServer is the chrony source shipped to every master/worker node via
	// MachineConfig. Empty means "use the bastion" (HTTPServer.IgnitionServerIP) —
	// the bastion is already reachable from every node and needs no extra
	// firewall opening.
	NTPServer string `json:"ntp_server,omitempty"`
}

// StaticIPConfig describes the static address plan for cluster nodes.
type StaticIPConfig struct {
	// Start is the bootstrap node's IP, not merely the first free address:
	// masters and workers allocate sequentially from Start+1, and the API
	// VIP derives from it (last octet .10) unless bastion.vip overrides.
	// It must not equal a live host such as the Proxmox host or the
	// ignition server — the bootstrap VM would ARP-fight it.
	Start string `json:"start"`
	// Netmask is derived from machine_cidr at load time; a YAML value is
	// overwritten. The field persists only to carry the dotted form
	// consumed by kernel args and HAProxy/dnsmasq templates.
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
	// HAEnabled provisions Proxmox HA-manager anti-affinity for master VMs
	// (requires PVE 9+; see infrastructure/terraform/modules/proxmox-okd/ha.tf).
	HAEnabled bool `json:"ha_enabled,omitempty"`
	// ControlPlaneNodes and WorkerNodes assign each VM by index to a
	// Proxmox node. A list shorter than the group's topology count pads
	// with Node for the remaining VMs; a longer list fails validation.
	// Downstream keeps OKD's "master" vocabulary: these render into the
	// master_target_nodes / worker_target_nodes terraform vars.
	ControlPlaneNodes []string `json:"control_plane_nodes,omitempty"`
	WorkerNodes       []string `json:"worker_nodes,omitempty"`
	// SSHHostFingerprint pins the Proxmox host's SSH key in standard
	// SHA256:<base64> format (from ssh-keygen -lf or the Proxmox UI). When
	// set, every SSH connection is verified and refused on mismatch. When
	// unset, accept-new TOFU applies and the observed fingerprints are
	// logged at WARN so the operator can pin one.
	SSHHostFingerprint string `json:"ssh_host_fingerprint,omitempty"`
	// RequirePinnedFingerprint fails closed when SSHHostFingerprint is not
	// set: sshpin.Verify returns an AuthError instead of the WARN+TOFU
	// fallback. Intended for security-sensitive deploys. Default false
	// preserves existing accept-new behaviour.
	RequirePinnedFingerprint bool `json:"require_pinned_fingerprint,omitempty"`
}

// redactedProxmoxConfig is the safe projection of ProxmoxConfig returned by
// Redacted(). It omits Username, Password, and APIToken so a slog.Any call
// carrying a *ProxmoxConfig cannot reach those fields through RedactHandler's
// interface dispatch.
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

// Redacted returns a struct containing only the non-credential fields of p,
// satisfying the interface{ Redacted() any } that logutil.redactAny detects.
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
	Port             int    `json:"port"`
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
	return "production"
}

// DisksConfig sizes the optional extra data disk (Ceph/storage) attached to
// each node in a group. The root/OS disk is sized by topology.<group>.disk_gb
// (NodeConfig.DiskGB); per-VM placement lives in provider.proxmox.
type DisksConfig struct {
	WorkerDataSizeGB       int `json:"worker_data_size_gb"`
	ControlPlaneDataSizeGB int `json:"control_plane_data_size_gb"`
}
