// Package steps provides wizard step implementations for the TUI.
package steps

// ═══════════════════════════════════════════════════════════════════════════════
// CENTRALIZED DEFAULT VALUES
// ═══════════════════════════════════════════════════════════════════════════════
const (
	DefaultClusterName = "mycluster"
	DefaultDomain      = "k8s.local"
)

const (
	DefaultControlPlaneCount = 3
	DefaultWorkerCount       = 3
)

const (
	DefaultControlPlaneCPU    = 4
	DefaultControlPlaneMemory = 12288 // MB
	DefaultControlPlaneDisk   = 50    // GB
)

const (
	DefaultWorkerCPU    = 8
	DefaultWorkerMemory = 20480 // MB
	DefaultWorkerDisk   = 50    // GB
)

const (
	DefaultOSDiskGB   = 50
	DefaultDataDiskGB = 500 // For Ceph
)

const (
	DefaultMachineCIDR = "192.168.1.0/24"
	DefaultGateway     = "192.168.1.1"
	DefaultDNS         = "192.168.1.1"
	DefaultPodCIDR     = "10.128.0.0/14"
	DefaultServiceCIDR = "172.30.0.0/16"
	DefaultHostPrefix  = 23
	DefaultStartIP     = "192.168.1.100"
	DefaultNetmask     = "255.255.255.0"
	DefaultInterface   = "ens18"
	DefaultBastionIP   = "192.168.1.20"
	DefaultMetalLBPool = "192.168.1.205-192.168.1.230"
)

const (
	DefaultProxmoxBridge  = "vmbr0"
	DefaultProxmoxStorage = "local-lvm"
)

const (
	DefaultIgnitionServerIP = "192.168.1.20"
	DefaultHTTPPort         = 8080
	DefaultWebRoot          = "/var/www/html"
)

const (
	DefaultVMIDBase          = 6000
	DefaultBootstrapTimeout  = 3600  // 1 hour in seconds
	DefaultInstallTimeout    = 7200  // 2 hours in seconds
)

const (
	DefaultGitOpsProvider = "flux"
	DefaultGitOpsBranch   = "main"
	DefaultGitOpsPath     = "kubernetes/clusters/production"
)

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
