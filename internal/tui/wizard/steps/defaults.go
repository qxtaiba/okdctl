package steps

import "github.com/qxtaiba/okdctl/internal/addon/catalog/flux"

// Cluster identity defaults.
const (
	DefaultClusterName = "mycluster"
	DefaultDomain      = "k8s.local"
)

// Topology count defaults.
const (
	DefaultControlPlaneCount = 3
	DefaultWorkerCount       = 3
)

// Control-plane resource defaults.
const (
	DefaultControlPlaneCPU    = 4
	DefaultControlPlaneMemory = 12288 // MB
	DefaultControlPlaneDisk   = 50    // GB
)

// Worker resource defaults.
const (
	DefaultWorkerCPU    = 8
	DefaultWorkerMemory = 20480 // MB
	DefaultWorkerDisk   = 50    // GB
)

// Disk size defaults.
const (
	DefaultOSDiskGB   = 50
	DefaultDataDiskGB = 500 // For Ceph
)

// Networking defaults.
const (
	DefaultMachineCIDR = "192.168.1.0/24"
	DefaultGateway     = "192.168.1.1"
	DefaultDNS         = "192.168.1.1"
	DefaultPodCIDR     = "10.128.0.0/14"
	DefaultServiceCIDR = "172.30.0.0/16"
	DefaultHostPrefix  = 23
	// DefaultStartIP stays clear of the default proxmox host (.100) —
	// the bootstrap VM boots at start and must not ARP-fight it.
	DefaultStartIP   = "192.168.1.140"
	DefaultNetmask   = "255.255.255.0"
	DefaultInterface = "ens18"
	DefaultBastionIP = "192.168.1.20"
)

// Proxmox provider defaults.
const (
	DefaultProxmoxBridge  = "vmbr0"
	DefaultProxmoxStorage = "local-lvm"
)

// HTTP server defaults for ignition hosting.
const (
	DefaultIgnitionServerIP = "192.168.1.20"
	DefaultHTTPPort         = 443
	DefaultWebRoot          = "/var/www/html"
)

// VM numbering and deployment timeout defaults.
const (
	DefaultVMIDBase         = 6000
	DefaultBootstrapTimeout = 3600 // 1 hour in seconds
	DefaultInstallTimeout   = 7200 // 2 hours in seconds
)

// Addon defaults (GitOps).
const (
	DefaultGitOpsProvider = flux.ProviderID
	DefaultGitOpsBranch   = "main"
	DefaultGitOpsPath     = "kubernetes/clusters/production"
)

// Validator bounds applied to wizard input fields.
const (
	MinNodeCount  = 1
	MaxNodeCount  = 100
	MinCPU        = 1
	MaxCPU        = 128
	MinMemoryMB   = 1024
	MaxMemoryMB   = 1048576 // 1TB
	MinOSDiskGB   = 20
	MaxOSDiskGB   = 1000
	MinDataDiskGB = 50
	MaxDataDiskGB = 5000
	MinVMID       = 100
	MaxVMID       = 999999999
	MinTimeout    = 60
	MaxTimeout    = 86400 // 24 hours
	MinHostPrefix = 20
	MaxHostPrefix = 28
	MinPort       = 1
	MaxPort       = 65535
)
