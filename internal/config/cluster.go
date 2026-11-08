// Package config defines the okdctl.yaml schema (v3), its loader and its
// validators. Serialization uses sigs.k8s.io/yaml, mapping json struct tags
// to YAML keys.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// SchemaVersionCurrent is the only schemaVersion this release reads.
const SchemaVersionCurrent = "v3"

// Config is the root okdctl.yaml schema.
type Config struct {
	SchemaVersion string                  `json:"schemaVersion"`
	Cluster       ClusterConfig           `json:"cluster"`
	Distribution  DistributionConfig      `json:"distribution"`
	Provider      ProviderConfig          `json:"provider"`
	Networking    NetworkingConfig        `json:"networking"`
	ControlPlane  NodeDefaults            `json:"control_plane"`
	Workers       WorkersConfig           `json:"workers"`
	Nodes         map[string]NodeOverride `json:"nodes,omitempty"`
	Files         FilesConfig             `json:"files"`
	Lifecycle     LifecycleConfig         `json:"lifecycle"`
	Handoff       HandoffConfig           `json:"handoff"`
	Defaults      DefaultsConfig          `json:"defaults"`
}

// ClusterConfig names the cluster and its base domain.
type ClusterConfig struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// DistributionConfig selects the Kubernetes distribution and release.
type DistributionConfig struct {
	Type    DistributionType `json:"type"`
	Version string           `json:"version"`
}

// ProviderConfig selects the infrastructure provider and its settings.
type ProviderConfig struct {
	Type    ProviderType  `json:"type"`
	Proxmox ProxmoxConfig `json:"proxmox"`
}

// ProxmoxConfig configures the Proxmox VE provider; the API token and the
// insecure flag live in okdctl.env, never here.
type ProxmoxConfig struct {
	Endpoint   string   `json:"endpoint"`
	Nodes      []string `json:"nodes"`
	Storage    string   `json:"storage"`
	ISOStorage string   `json:"iso_storage"`
	Bridge     string   `json:"bridge"`
	VLAN       int      `json:"vlan"`
	CPUType    string   `json:"cpu_type"`
	Pool       string   `json:"pool"`
	HA         bool     `json:"ha"`
	VMIDBase   int      `json:"vmid_base"`
}

// NetworkingConfig lays out the machine network, the VIPs, the node address
// range and the cluster-internal CIDRs.
type NetworkingConfig struct {
	MachineCIDR string   `json:"machine_cidr"`
	Gateway     string   `json:"gateway"`
	DNS         []string `json:"dns"`
	APIVIP      string   `json:"api_vip"`
	IngressVIP  string   `json:"ingress_vip"`
	NodeIPStart string   `json:"node_ip_start"`
	NTP         []string `json:"ntp"`
	PodCIDR     string   `json:"pod_cidr"`
	ServiceCIDR string   `json:"service_cidr"`
	HostPrefix  int      `json:"host_prefix"`
}

// NodeDefaults sizes every node of a role; Disks[0] is the root disk.
type NodeDefaults struct {
	CPU       int        `json:"cpu"`
	MemoryMiB int        `json:"memory_mib"`
	Disks     []DiskSpec `json:"disks"`
}

// WorkersConfig sizes the worker pool; Count is how many, indices come from
// the nodes overrides or are filled from zero.
type WorkersConfig struct {
	Count int `json:"count"`
	NodeDefaults
}

// NodeOverride replaces role defaults for one named node; nil fields inherit.
type NodeOverride struct {
	CPU       *int       `json:"cpu,omitempty"`
	MemoryMiB *int       `json:"memory_mib,omitempty"`
	Disks     []DiskSpec `json:"disks,omitempty"`
	Host      string     `json:"host,omitempty"`
	IP        string     `json:"ip,omitempty"`
}

// DiskSpec describes one virtual disk; an empty Storage uses provider.proxmox.storage.
type DiskSpec struct {
	SizeGiB int    `json:"size_gib"`
	Storage string `json:"storage,omitempty"`
	Wipe    *bool  `json:"wipe,omitempty"`
}

// FilesConfig points at the pull secret and SSH public key baked into the ISO.
type FilesConfig struct {
	PullSecret   string `json:"pull_secret"`
	SSHPublicKey string `json:"ssh_public_key"`
}

// LifecycleConfig tunes rolling node operations; durations read and write
// Go duration strings such as 15m.
type LifecycleConfig struct {
	BeforeNextNode Hook
	DrainTimeout   time.Duration
}

// Hook is a command run between node operations; an empty Command disables it.
type Hook struct {
	Command string
	Timeout time.Duration
}

// HandoffConfig toggles applying handoff/secrets and handoff/manifests after install.
type HandoffConfig struct {
	Enabled bool `json:"enabled"`
}

// DefaultsConfig holds cluster-wide switches applied at install time.
type DefaultsConfig struct {
	DisableRedHatCatalogs bool `json:"disable_redhat_catalogs"`
}

type lifecycleWire struct {
	BeforeNextNode hookWire `json:"before_next_node"`
	DrainTimeout   string   `json:"drain_timeout"`
}

type hookWire struct {
	Command string `json:"command"`
	Timeout string `json:"timeout"`
}

// MarshalJSON writes durations as Go duration strings.
func (l LifecycleConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(lifecycleWire{
		BeforeNextNode: hookWire{Command: l.BeforeNextNode.Command, Timeout: l.BeforeNextNode.Timeout.String()},
		DrainTimeout:   l.DrainTimeout.String(),
	})
}

// UnmarshalJSON parses durations from Go duration strings and refuses unknown keys.
func (l *LifecycleConfig) UnmarshalJSON(data []byte) error {
	var w lifecycleWire
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return err
	}
	timeout, err := parseDuration("lifecycle.before_next_node.timeout", w.BeforeNextNode.Timeout)
	if err != nil {
		return err
	}
	drain, err := parseDuration("lifecycle.drain_timeout", w.DrainTimeout)
	if err != nil {
		return err
	}
	l.BeforeNextNode = Hook{Command: w.BeforeNextNode.Command, Timeout: timeout}
	l.DrainTimeout = drain
	return nil
}

func parseDuration(field, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q (use Go syntax such as 15m)", field, value)
	}
	return d, nil
}
