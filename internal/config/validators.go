package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

var (
	dnsLabelPattern   = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	domainPattern     = regexp.MustCompile(`^([a-z0-9]([-a-z0-9]*[a-z0-9])?\.)*[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	okdVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+-okd-[a-zA-Z0-9.-]+$`)

	interfaceNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*$`)
	proxmoxNamePattern   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
	// PVE storage IDs allow dots ([A-Za-z][A-Za-z0-9\-_.]*), unlike node names.
	proxmoxStorageNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*$`)
	proxmoxCPUTypePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_+.,=-]*$`)
)

func validateRequired(cfg *Config, result *ValidationResult) {
	checks := []struct {
		bad   bool
		field string
		msg   string
	}{
		{cfg.Cluster.Name == "", FieldClusterName, "cluster name is required"},
		{cfg.Cluster.Domain == "", FieldClusterDomain, "cluster domain is required"},
		{cfg.Distribution.Type == "", FieldDistributionType, "distribution type is required"},
		{cfg.Distribution.Version == "", FieldDistributionVersion, "distribution version is required"},
		{cfg.Provider.Type == "", FieldProviderType, "provider type is required"},
		{cfg.Topology.ControlPlane.Count < 1, FieldTopologyControlPlaneCount, "must have at least 1 control plane node"},
		{cfg.Networking.MachineCIDR == "", FieldNetworkingMachineCIDR, "machine CIDR is required"},
		{cfg.Networking.PodCIDR == "", FieldNetworkingPodCIDR, "pod CIDR is required"},
		{cfg.Networking.ServiceCIDR == "", FieldNetworkingServiceCIDR, "service CIDR is required"},
		{cfg.Networking.Gateway == "", FieldNetworkingGateway, "gateway is required"},
		{len(cfg.Networking.DNS) == 0, FieldNetworkingDNS, "at least one DNS server is required"},
	}
	for _, c := range checks {
		if c.bad {
			result.AddError(c.field, c.msg)
		}
	}
}

func validateEnums(cfg *Config, result *ValidationResult) {
	if cfg.Cluster.Name != "" {
		if err := ValidateClusterName(cfg.Cluster.Name); err != nil {
			result.AddError(FieldClusterName, err.Error())
		}
	}

	if cfg.Cluster.Domain != "" {
		if err := ValidateDomain(cfg.Cluster.Domain); err != nil {
			result.AddError(FieldClusterDomain, err.Error())
		}
	}

	if cfg.Distribution.Type != "" && !isValidDistribution(cfg.Distribution.Type) {
		result.AddError(FieldDistributionType, fmt.Sprintf("unsupported distribution: %s", cfg.Distribution.Type))
	}

	if cfg.Provider.Type != "" && !isValidProvider(cfg.Provider.Type) {
		result.AddError(FieldProviderType, fmt.Sprintf("unsupported provider: %s", cfg.Provider.Type))
	}

	if env := cfg.Deployment.TerraformEnv; env != "" {
		if err := ValidateTerraformEnv(env); err != nil {
			result.AddError(FieldDeploymentTerraformEnv, err.Error())
		}
	}
}

// validateTerraformEnvDir requires a matching environment directory under
// projectRoot. "production" ships in the repo and is trusted without a disk
// check so DefaultConfig()-based callers (including tests run from a
// package-local CWD) never trip this; a genuinely custom environment must
// have a matching directory or terraform fails deep in the install phase
// instead of here. An empty projectRoot resolves against the process cwd,
// which only matches materialization when the caller runs at the workspace
// root — deploy's gate passes its resolved root explicitly.
func validateTerraformEnvDir(cfg *Config, projectRoot string, result *ValidationResult) {
	env := cfg.Deployment.TerraformEnv
	// A pattern-invalid env already fails validateEnums; joining it into a
	// path here would only add a second, noisier error.
	if env == "" || env == defaultTerraformEnv || ValidateTerraformEnv(env) != nil {
		return
	}
	dir := workspace.TerraformEnvDir(projectRoot, env)
	if !system.DirExists(dir) {
		result.AddError(FieldDeploymentTerraformEnv, fmt.Sprintf("no environment directory at %s", dir))
	}
}

func checkCIDROverlap(cidr1, cidr2, field, otherName string, result *ValidationResult) {
	if !IsValidCIDR(cidr1) || !IsValidCIDR(cidr2) {
		return
	}
	overlap, err := netutil.CIDRsOverlap(cidr1, cidr2)
	if err != nil {
		result.AddError(field, fmt.Sprintf("cannot check overlap with %s: %v", otherName, err))
	} else if overlap {
		result.AddError(field, "overlaps with "+otherName)
	}
}

func checkIPInCIDR(ip, cidr, field string, result *ValidationResult) {
	if ip == "" || !IsValidIP(ip) || !IsValidCIDR(cidr) {
		return
	}
	ok, err := netutil.IPInCIDR(ip, cidr)
	if err != nil {
		result.AddError(field, fmt.Sprintf("cannot check CIDR membership: %v", err))
	} else if !ok {
		result.AddError(field, fmt.Sprintf("must be within machine CIDR %s", cidr))
	}
}

func checkNodeResources(node NodeConfig, minCPU, minMemory, minDisk int, cpuField, memField, diskField, label string, result *ValidationResult) {
	if node.CPU < minCPU {
		result.AddError(cpuField, fmt.Sprintf("must have at least %d vCPUs for %s", minCPU, label))
	}
	if node.MemoryMB < minMemory {
		result.AddError(memField, fmt.Sprintf("must have at least %d MB of memory for %s", minMemory, label))
	}
	if node.DiskGB < minDisk {
		result.AddError(diskField, fmt.Sprintf("must have at least %d GB of disk space for %s", minDisk, label))
	}
}

func validateNetworking(cfg *Config, result *ValidationResult) {
	if cfg.Networking.MachineCIDR != "" && !IsValidCIDR(cfg.Networking.MachineCIDR) {
		result.AddError(FieldNetworkingMachineCIDR, "must be a valid CIDR notation")
	}

	if cfg.Networking.PodCIDR != "" && !IsValidCIDR(cfg.Networking.PodCIDR) {
		result.AddError(FieldNetworkingPodCIDR, "must be a valid CIDR notation")
	}

	if cfg.Networking.ServiceCIDR != "" && !IsValidCIDR(cfg.Networking.ServiceCIDR) {
		result.AddError(FieldNetworkingServiceCIDR, "must be a valid CIDR notation")
	}

	if cfg.Networking.Gateway != "" && !IsValidIP(cfg.Networking.Gateway) {
		result.AddError(FieldNetworkingGateway, "must be a valid IP address")
	}

	for i, dns := range cfg.Networking.DNS {
		if !IsValidIP(dns) {
			result.AddError(fmt.Sprintf("%s[%d]", FieldNetworkingDNS, i), "must be a valid IP address")
		}
	}

	if cfg.Networking.NTPServer != "" && !isValidHostOrIP(cfg.Networking.NTPServer) {
		result.AddError(FieldNetworkingNTPServer, "must be a valid hostname or IP address")
	}

	podCIDR := cfg.Networking.PodCIDR
	serviceCIDR := cfg.Networking.ServiceCIDR
	machineCIDR := cfg.Networking.MachineCIDR

	checkCIDROverlap(podCIDR, serviceCIDR, FieldNetworkingPodCIDR, "service CIDR", result)
	checkCIDROverlap(podCIDR, machineCIDR, FieldNetworkingPodCIDR, "machine CIDR", result)
	checkCIDROverlap(serviceCIDR, machineCIDR, FieldNetworkingServiceCIDR, "machine CIDR", result)
}

func validateAdvancedNetworking(cfg *Config, result *ValidationResult) {
	machineCIDR := cfg.Networking.MachineCIDR
	gateway := cfg.Networking.Gateway
	bastionIP := cfg.Networking.Bastion.IP
	staticIPStart := cfg.Networking.StaticIP.Start

	if bastionIP != "" && !IsValidIP(bastionIP) {
		result.AddError(FieldNetworkingBastionIP, "must be a valid IP address")
	}
	if staticIPStart != "" && !IsValidIP(staticIPStart) {
		result.AddError(FieldNetworkingStaticIPStart, "must be a valid IP address")
	}

	if machineCIDR == "" || !IsValidCIDR(machineCIDR) {
		return
	}

	checkIPInCIDR(gateway, machineCIDR, FieldNetworkingGateway, result)
	checkIPInCIDR(bastionIP, machineCIDR, FieldNetworkingBastionIP, result)
	checkIPInCIDR(staticIPStart, machineCIDR, FieldNetworkingStaticIPStart, result)

	if cfg.Networking.Bastion.VIP != "" {
		if !IsValidIP(cfg.Networking.Bastion.VIP) {
			result.AddError("networking.bastion.vip", "must be a valid IP address")
		} else {
			checkIPInCIDR(cfg.Networking.Bastion.VIP, machineCIDR, "networking.bastion.vip", result)
			if cfg.Networking.Bastion.VIP == gateway {
				result.AddError("networking.bastion.vip", "vip cannot be the same as the gateway")
			}
			if cfg.Networking.Bastion.VIP == bastionIP {
				result.AddError("networking.bastion.vip", "vip cannot be the same as the bastion ip")
			}
		}
	}

	if staticIPStart != "" {
		netmask := cfg.Networking.StaticIP.Netmask
		expected, expectedErr := netutil.CIDRToNetmask(machineCIDR)
		switch {
		case netmask == "":
			result.AddError(FieldNetworkingStaticIPNetmask, "netmask is required when using static IPs")
		case !isValidNetmask(netmask):
			result.AddError(FieldNetworkingStaticIPNetmask, "must be a valid netmask (e.g., 255.255.255.0 or /24)")
		case expectedErr == nil && !netmaskMatches(netmask, expected):
			result.AddError(FieldNetworkingStaticIPNetmask,
				fmt.Sprintf("must match machine CIDR %s (expected %s); netmask is derived from machine_cidr", machineCIDR, expected))
		}

		iface := cfg.Networking.StaticIP.Interface
		if iface == "" {
			result.AddError(FieldNetworkingStaticIPIface, "interface name is required when using static IPs")
		} else if !interfaceNamePattern.MatchString(iface) {
			result.AddError(FieldNetworkingStaticIPIface, "must be a valid interface name (e.g., eth0, ens18)")
		}
	}

	// One physical bastion host runs dnsmasq (the DNS server baked into
	// every node's static-ip kernel args, see setup/kargs.go) and the
	// ignition HTTPS server (apache binds HTTPServer.IgnitionServerIP
	// directly, see setup/apache.go). A hand-edited config that points
	// either at a different host than networking.bastion.ip fails deep in
	// setup with no config-time diagnostic otherwise.
	if bastionIP != "" {
		if dns := cfg.Networking.StaticIP.DNS; dns != "" && dns != bastionIP {
			result.AddError(FieldNetworkingStaticIPDNS, "must match networking.bastion.ip — dnsmasq (the VMs' DNS server) runs on the bastion")
		}
		if ignitionIP := cfg.HTTPServer.IgnitionServerIP; ignitionIP != "" && ignitionIP != bastionIP {
			result.AddError(FieldHTTPServerIP, "must match networking.bastion.ip — the ignition http server binds to the bastion")
		}
	}

	type namedIP struct {
		name string
		ip   string
	}
	uniqueIPs := []namedIP{
		{"gateway", gateway},
		{"bastion.ip", bastionIP},
	}

	seen := make(map[netip.Addr]string)
	for _, nip := range uniqueIPs {
		if nip.ip == "" {
			continue
		}
		addr, err := netip.ParseAddr(nip.ip)
		if err != nil {
			continue
		}
		if prevField, exists := seen[addr]; exists {
			result.AddError(fmt.Sprintf("networking.%s", nip.name),
				fmt.Sprintf("ip %s is already used by %s", nip.ip, prevField))
		} else {
			seen[addr] = nip.name
		}
	}
}

// validateStaticIPCollisions rejects a static_ip.start equal to the Proxmox
// host or ignition server IP. The bootstrap node boots at start (see
// StaticIPConfig.Start), so an equal address ARP-fights a live host on the
// machine network.
func validateStaticIPCollisions(cfg *Config, result *ValidationResult) {
	start, err := netip.ParseAddr(cfg.Networking.StaticIP.Start)
	if err != nil {
		return
	}
	if host, err := netip.ParseAddr(proxmoxHostAddr(cfg.Provider.Proxmox)); err == nil && host == start {
		result.AddError(FieldNetworkingStaticIPStart,
			fmt.Sprintf("must not equal the proxmox host ip %s (the bootstrap node boots at static_ip.start)", host))
	}
	if ign, err := netip.ParseAddr(cfg.HTTPServer.IgnitionServerIP); err == nil && ign == start {
		result.AddError(FieldNetworkingStaticIPStart,
			fmt.Sprintf("must not equal the ignition server ip %s (the bootstrap node boots at static_ip.start)", ign))
	}
}

func proxmoxHostAddr(proxmox *ProxmoxConfig) string {
	if proxmox == nil {
		return ""
	}
	host := strings.TrimPrefix(proxmox.Host, "http://")
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	return host
}

func netmaskMatches(netmask, dotted string) bool {
	if strings.HasPrefix(netmask, "/") {
		converted, err := netutil.CIDRToNetmask("0.0.0.0" + netmask)
		return err == nil && converted == dotted
	}
	return netmask == dotted
}

func validateResources(cfg *Config, result *ValidationResult) {
	minMemory := getMinMemoryForDistribution(cfg.Distribution.Type)
	checkNodeResources(cfg.Topology.ControlPlane, MinCPUGeneric, minMemory, MinDiskGBGeneric,
		FieldTopologyControlPlaneCPU, FieldTopologyControlPlaneMemory, FieldTopologyControlPlaneDisk,
		string(cfg.Distribution.Type), result)
	checkNodeResourceCaps(cfg.Topology.ControlPlane,
		FieldTopologyControlPlaneCPU, FieldTopologyControlPlaneMemory, FieldTopologyControlPlaneDisk, result)

	if cfg.Topology.Workers.Count > 0 {
		checkNodeResources(cfg.Topology.Workers, MinCPUGeneric, MinMemoryMBGeneric, MinDiskGBGeneric,
			FieldTopologyWorkersCPU, FieldTopologyWorkersMemory, FieldTopologyWorkersDisk,
			"workers", result)
		checkNodeResourceCaps(cfg.Topology.Workers,
			FieldTopologyWorkersCPU, FieldTopologyWorkersMemory, FieldTopologyWorkersDisk, result)
	}

	if err := nodeCountBounds.check(cfg.Topology.ControlPlane.Count); err != nil {
		result.AddError(FieldTopologyControlPlaneCount, err.Error())
	}
	if err := nodeCountBounds.check(cfg.Topology.Workers.Count); err != nil {
		result.AddError(FieldTopologyWorkersCount, err.Error())
	}
	if err := dataDiskBounds.check(cfg.Disks.WorkerDataSizeGB); err != nil {
		result.AddError(FieldDisksWorkerDataSize, err.Error())
	}
	if err := dataDiskBounds.check(cfg.Disks.ControlPlaneDataSizeGB); err != nil {
		result.AddError(FieldDisksControlPlaneDataSize, err.Error())
	}
}

// checkNodeResourceCaps applies the wizard's upper bounds; floors are
// distribution-specific and belong to checkNodeResources.
func checkNodeResourceCaps(node NodeConfig, cpuField, memField, diskField string, result *ValidationResult) {
	if node.CPU > cpuBounds.hi {
		result.AddError(cpuField, fmt.Sprintf("maximum %d%s", cpuBounds.hi, cpuBounds.unit))
	}
	if node.MemoryMB > memoryBounds.hi {
		result.AddError(memField, fmt.Sprintf("maximum %d%s", memoryBounds.hi, memoryBounds.unit))
	}
	if node.DiskGB > osDiskBounds.hi {
		result.AddError(diskField, fmt.Sprintf("maximum %d%s", osDiskBounds.hi, osDiskBounds.unit))
	}
}

// validateDeployment mirrors the wizard's advanced-step constraints for
// hand-edited configs. Zero timeouts are valid — install falls back to its
// compiled defaults. BinDir is checked after ~-expansion because that is
// the form ResolveBinDir consumes; a value it would silently discard fails
// here instead.
func validateDeployment(cfg *Config, result *ValidationResult) {
	if v := cfg.Deployment.BootstrapTimeout; v != 0 {
		if err := timeoutBounds.check(v); err != nil {
			result.AddError(FieldDeploymentBootstrapTimeout, err.Error())
		}
	}
	if v := cfg.Deployment.InstallTimeout; v != 0 {
		if err := timeoutBounds.check(v); err != nil {
			result.AddError(FieldDeploymentInstallTimeout, err.Error())
		}
	}
	if v := cfg.Deployment.BinDir; v != "" {
		if err := ValidateBinDir(system.ExpandPath(v)); err != nil {
			result.AddError(FieldDeploymentBinDir, err.Error())
		}
	}
}

func validateProvider(cfg *Config, result *ValidationResult) {
	if cfg.Provider.Type == ProviderProxmox {
		validateProxmoxConfig(cfg.Provider.Proxmox, result)
		validatePlacementCounts(cfg, result)
		validateVMIDBase(cfg, result)
	}
}

// validateVMIDBase bounds topology.vm_id_base at config load (vmidBounds,
// the same range ValidateVMID enforces in the wizard): terraform computes
// each vmid as base + index, so an out-of-range base otherwise fails deep
// inside the apply instead of here. The overflow check keeps the highest
// computed vmid (bootstrap + masters + workers) under the Proxmox ceiling.
func validateVMIDBase(cfg *Config, result *ValidationResult) {
	base := cfg.Topology.VMIDBase
	if err := vmidBounds.check(base); err != nil {
		result.AddError(FieldTopologyVMIDBase, err.Error())
		return
	}
	nodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	if base > vmidBounds.hi-nodes {
		result.AddError(FieldTopologyVMIDBase,
			fmt.Sprintf("plus %d node vmids exceeds the maximum vmid %d", nodes, vmidBounds.hi))
	}
}

// validatePlacementCounts rejects placement lists longer than the group's
// topology count: terraform consumes entries by index and silently ignores
// the excess, so a longer list is an operator error. Shorter lists are valid
// — unassigned VMs fall back to provider.proxmox.node.
func validatePlacementCounts(cfg *Config, result *ValidationResult) {
	proxmox := cfg.Provider.Proxmox
	if proxmox == nil {
		return
	}
	if n := len(proxmox.ControlPlaneNodes); n > cfg.Topology.ControlPlane.Count {
		result.AddError(FieldProxmoxControlPlaneNodes,
			fmt.Sprintf("has %d entries but %s is %d", n, FieldTopologyControlPlaneCount, cfg.Topology.ControlPlane.Count))
	}
	if n := len(proxmox.WorkerNodes); n > cfg.Topology.Workers.Count {
		result.AddError(FieldProxmoxWorkerNodes,
			fmt.Sprintf("has %d entries but %s is %d", n, FieldTopologyWorkersCount, cfg.Topology.Workers.Count))
	}
}

func validateProxmoxConfig(proxmox *ProxmoxConfig, result *ValidationResult) {
	if proxmox == nil {
		result.AddError(FieldProviderProxmox, "proxmox configuration is required when using proxmox provider")
		return
	}

	switch {
	case proxmox.Host == "":
		result.AddError(FieldProxmoxHost, "proxmox host is required")
	case strings.HasPrefix(proxmox.Host, "http://") && !proxmox.InsecureHTTP:
		result.AddError(FieldProxmoxHost,
			"http:// endpoint transmits credentials in plaintext; set provider.proxmox.insecure_http: true to opt in")
	default:
		host := strings.TrimPrefix(proxmox.Host, "http://")
		if strings.Contains(host, ":") {
			h, _, err := net.SplitHostPort(host)
			if err != nil {
				result.AddError(FieldProxmoxHost, "must be a valid host or host:port")
			} else {
				host = h
			}
		}
		if host != "" && !isValidHostOrIP(host) {
			result.AddError(FieldProxmoxHost, "must be a valid hostname or IP address")
		}
	}

	if proxmox.Node == "" {
		result.AddError(FieldProxmoxNode, "proxmox node name is required")
	} else if !proxmoxNamePattern.MatchString(proxmox.Node) {
		result.AddError(FieldProxmoxNode, "must be a valid Proxmox node name (alphanumeric, hyphens, underscores)")
	}

	if proxmox.Storage == "" {
		result.AddError(FieldProxmoxStorage, "proxmox storage is required")
	} else if !proxmoxNamePattern.MatchString(proxmox.Storage) {
		result.AddError(FieldProxmoxStorage, "must be a valid Proxmox storage name (alphanumeric, hyphens, underscores)")
	}

	if proxmox.ISOStorage != "" && !proxmoxStorageNamePattern.MatchString(proxmox.ISOStorage) {
		result.AddError(FieldProxmoxISOStorage, "must be a valid Proxmox storage name (alphanumeric, dots, hyphens, underscores)")
	}

	if proxmox.DataStorage != "" && !proxmoxStorageNamePattern.MatchString(proxmox.DataStorage) {
		result.AddError(FieldProxmoxDataStorage, "must be a valid Proxmox storage name (alphanumeric, dots, hyphens, underscores)")
	}

	if proxmox.Bridge != "" && !interfaceNamePattern.MatchString(proxmox.Bridge) {
		result.AddError(FieldProxmoxBridge, "must be a valid network interface name (e.g., vmbr0, vmbr1)")
	}

	if proxmox.CPUType != "" && !proxmoxCPUTypePattern.MatchString(proxmox.CPUType) {
		result.AddError(FieldProxmoxCPUType, "must contain only alphanumeric characters, hyphens, underscores, plus, dot, comma, or equals")
	}

	for i, node := range proxmox.ControlPlaneNodes {
		if node != "" && !proxmoxNamePattern.MatchString(node) {
			result.AddError(fmt.Sprintf("%s[%d]", FieldProxmoxControlPlaneNodes, i), "must be a valid Proxmox node name")
		}
	}
	for i, node := range proxmox.WorkerNodes {
		if node != "" && !proxmoxNamePattern.MatchString(node) {
			result.AddError(fmt.Sprintf("%s[%d]", FieldProxmoxWorkerNodes, i), "must be a valid Proxmox node name")
		}
	}

	if err := ValidateSSHFingerprint(proxmox.SSHHostFingerprint); err != nil {
		result.AddError("provider.proxmox.ssh_host_fingerprint", err.Error())
	}

	validateAdditionalNetworks(proxmox.AdditionalNetworks, result)
}

// additionalNetworkModels is the qemu NIC model allowlist for
// provider.proxmox.additional_networks[].model (empty selects the virtio
// default at render time).
var additionalNetworkModels = []string{"virtio", "e1000", "rtl8139", "vmxnet3"}

// validateAdditionalNetworks mirrors the primary Bridge validation for each
// extra NIC. Bridge and Model render into terraform.tfvars HCL where %q
// escaping does not neutralize ${…} interpolation, so the charset gates
// here are the injection boundary for the root-privileged terraform apply.
func validateAdditionalNetworks(networks []AdditionalNetwork, result *ValidationResult) {
	for i, n := range networks {
		field := fmt.Sprintf("provider.proxmox.additional_networks[%d]", i)
		if n.Bridge == "" {
			result.AddError(field+".bridge", "bridge is required for each additional network")
		} else if !interfaceNamePattern.MatchString(n.Bridge) {
			result.AddError(field+".bridge", "must be a valid network interface name (e.g., vmbr0, vmbr1)")
		}
		if n.Model != "" && !slices.Contains(additionalNetworkModels, n.Model) {
			result.AddError(field+".model", "must be one of virtio, e1000, rtl8139, vmxnet3")
		}
		if n.VLANTag < 0 || n.VLANTag > 4094 {
			result.AddError(field+".vlan_tag", "must be between 1 and 4094 (or omitted)")
		}
	}
}

// httpRootUnsafe holds characters that carry meaning in Apache config
// directives or a POSIX shell; any of them in a DocumentRoot value would
// allow the operator to inject directives or break out of quoted contexts.
const httpRootUnsafe = "\n\r\t \"'`$;<>\\"

func validateHTTPServer(cfg *Config, result *ValidationResult) {
	if cfg.HTTPServer.IgnitionServerIP != "" && !IsValidIP(cfg.HTTPServer.IgnitionServerIP) {
		result.AddError(FieldHTTPServerIP, "must be a valid IPv4 or IPv6 literal")
	}

	if cfg.HTTPServer.Root != "" {
		if !filepath.IsAbs(cfg.HTTPServer.Root) {
			result.AddError(FieldHTTPServerRoot, "must be an absolute path")
		} else if strings.ContainsAny(cfg.HTTPServer.Root, httpRootUnsafe) {
			result.AddError(FieldHTTPServerRoot, "must not contain shell or Apache directive metacharacters")
		}
	}
}

func validateDistribution(cfg *Config, result *ValidationResult) {
	if cfg.Distribution.Type == DistributionOKD {
		ValidateOKDConfig(cfg, result)
	}
}

func validateOKDVersion(version string) error {
	if version == "" {
		return fmt.Errorf("okd version is required")
	}

	if !okdVersionPattern.MatchString(version) {
		return fmt.Errorf("okd version must be in format X.Y.Z-okd-<suffix> (e.g., 4.22.0-okd-scos.7)")
	}

	return nil
}

// validateHAMasters requires master count to be odd for etcd quorum.
func validateHAMasters(count int) error {
	if count > 1 && count%2 == 0 {
		return fmt.Errorf("master replicas should be odd for ha quorum (1, 3, or 5), got %d", count)
	}
	return nil
}

// ValidateOKDConfig applies OKD-specific version and node-resource floors.
// Errors are appended to result; callers run this in addition to the
// distribution-neutral validators.
func ValidateOKDConfig(cfg *Config, result *ValidationResult) {
	if cfg.Distribution.Type == DistributionOKD {
		if err := validateOKDVersion(cfg.Distribution.Version); err != nil {
			result.AddError(FieldDistributionVersion, fmt.Sprintf("invalid okd version: %v", err))
		}

		checkNodeResources(cfg.Topology.ControlPlane, MinCPUControlPlaneOKD, MinMemoryMBControlPlaneOKD, MinDiskGBControlPlaneOKD,
			FieldTopologyControlPlaneCPU, FieldTopologyControlPlaneMemory, FieldTopologyControlPlaneDisk,
			"okd control plane", result)

		if err := validateHAMasters(cfg.Topology.ControlPlane.Count); err != nil {
			result.AddError(FieldTopologyControlPlaneCount, fmt.Sprintf("invalid master count: %v", err))
		}

		if cfg.Topology.Workers.Count > 0 {
			checkNodeResources(cfg.Topology.Workers, MinCPUWorkerOKD, MinMemoryMBWorkerOKD, MinDiskGBWorkerOKD,
				FieldTopologyWorkersCPU, FieldTopologyWorkersMemory, FieldTopologyWorkersDisk,
				"okd workers", result)
		}
	}
}

func validateFiles(cfg *Config, result *ValidationResult) {
	if cfg.Files.PullSecret != "" {
		path := system.ExpandPath(cfg.Files.PullSecret)
		if !system.FileExists(path) {
			result.AddError(FieldFilesPullSecret, "file does not exist: "+cfg.Files.PullSecret)
		}
	}

	if cfg.Files.SSHPublicKey != "" {
		path := system.ExpandPath(cfg.Files.SSHPublicKey)
		if !system.FileExists(path) {
			result.AddError(FieldFilesSSHPublicKey, "file does not exist: "+cfg.Files.SSHPublicKey)
		}
	}
}

// IsValidDNSLabel reports whether s is a valid RFC 1123 DNS label.
func IsValidDNSLabel(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	return dnsLabelPattern.MatchString(s)
}

func isValidDomain(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	return domainPattern.MatchString(s)
}

func isValidHostOrIP(s string) bool {
	return IsValidIP(s) || isValidDomain(s)
}

// IsValidIP reports whether s parses as an IPv4 or IPv6 literal.
func IsValidIP(s string) bool {
	_, err := netip.ParseAddr(s)
	return err == nil
}

// IsValidCIDR reports whether s parses as an IPv4 or IPv6 prefix.
func IsValidCIDR(s string) bool {
	_, err := netip.ParsePrefix(s)
	return err == nil
}

func isValidNetmask(s string) bool {
	if strings.HasPrefix(s, "/") {
		_, err := netip.ParsePrefix("0.0.0.0" + s)
		return err == nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return false
	}
	octets := addr.As4()
	mask := uint32(octets[0])<<24 | uint32(octets[1])<<16 | uint32(octets[2])<<8 | uint32(octets[3])
	// Reject 0.0.0.0 outright — it's a legal bit pattern but meaningless
	// as a host netmask (would claim the entire IPv4 space).
	if mask == 0 {
		return false
	}
	// A canonical netmask is N contiguous 1 bits followed by (32-N) zeros.
	// ^mask + 1 sets only the lowest zero-bit; a contiguous mask is a power
	// of two in (^mask + 1), equivalently (~m & (~m+1)) == (~m+1). We allow
	// the all-ones mask (255.255.255.255) where ~m is zero.
	inverted := ^mask
	return inverted == 0 || (inverted&(inverted+1)) == 0
}

func isValidDistribution(d DistributionType) bool {
	return slices.Contains(supportedDistributions(), d)
}

func isValidProvider(p ProviderType) bool {
	return slices.Contains(supportedProviders(), p)
}

func getMinMemoryForDistribution(d DistributionType) int {
	if d == DistributionOKD {
		return MinMemoryMBControlPlaneOKD
	}
	return DefaultMinMemoryMB
}

// ValidateClusterName returns a descriptive error if value violates the
// DNS-1123 cluster-name grammar (2-63 chars, lowercase a-z/0-9/-, no
// leading or trailing hyphen; a leading digit is allowed).
func ValidateClusterName(value string) error {
	if len(value) < 2 {
		return errors.New("must be at least 2 characters")
	}
	if !IsValidDNSLabel(value) {
		return errors.New("must contain only lowercase letters, numbers, and hyphens, not start or end with a hyphen, max 63 chars")
	}
	return nil
}

// ValidateDomain returns a descriptive error if value is not a
// dot-separated DNS name of reasonable length.
func ValidateDomain(value string) error {
	if len(value) < 3 {
		return errors.New("must be at least 3 characters")
	}
	if !isValidDomain(value) {
		return errors.New("invalid domain format")
	}
	return nil
}

// ValidateProxmoxNodeName rejects names that do not match
// [a-zA-Z][a-zA-Z0-9_-]*, the same constraint enforced by
// ValidateProxmoxConfig and the downstream pveshRun guard.
func ValidateProxmoxNodeName(value string) error {
	if !proxmoxNamePattern.MatchString(value) {
		return errors.New("must start with a letter and contain only alphanumeric, hyphens, or underscores")
	}
	return nil
}

// ValidateProxmoxHost accepts a hostname, IP, or host:port.
func ValidateProxmoxHost(value string) error {
	host := value
	if strings.Contains(value, ":") {
		h, _, err := net.SplitHostPort(value)
		if err != nil {
			return errors.New("invalid host:port format")
		}
		host = h
	}
	if host == "" || !isValidHostOrIP(host) {
		return errors.New("must be a valid hostname or IP address")
	}
	return nil
}

// ValidateNTPServer accepts an empty string (the bastion default applies)
// or a hostname/IP for the chrony source shipped in the master/worker
// MachineConfigs.
func ValidateNTPServer(value string) error {
	if value == "" {
		return nil
	}
	if !isValidHostOrIP(value) {
		return errors.New("must be a valid hostname or IP address")
	}
	return nil
}

// ValidateIP returns "invalid ip address" if value does not parse as an
// IPv4 or IPv6 literal.
func ValidateIP(value string) error {
	if !IsValidIP(value) {
		return errors.New("invalid ip address")
	}
	return nil
}

// ValidateGatewayInCIDR reports an error if gateway is not inside cidr.
// When either value is missing or malformed this returns nil — the required
// field validators surface those cases separately.
func ValidateGatewayInCIDR(gateway, cidr string) error {
	if gateway == "" || !IsValidIP(gateway) || cidr == "" || !IsValidCIDR(cidr) {
		return nil
	}
	ok, err := netutil.IPInCIDR(gateway, cidr)
	if err != nil {
		return fmt.Errorf("cannot check gateway CIDR membership: %w", err)
	}
	if !ok {
		return fmt.Errorf("gateway %s is not within machine CIDR %s", gateway, cidr)
	}
	return nil
}

// ValidateCIDR returns "invalid cidr format" if value does not parse as
// an IPv4 or IPv6 prefix.
func ValidateCIDR(value string) error {
	if !IsValidCIDR(value) {
		return errors.New("invalid cidr format (e.g., 192.168.1.0/24)")
	}
	return nil
}

// intBounds is the single encoding of an integer field's [lo, hi] range,
// shared by the wizard string validators and the file-load validators so
// the two surfaces cannot drift.
type intBounds struct {
	lo, hi int
	unit   string
}

func (b intBounds) check(n int) error {
	if n < b.lo {
		return fmt.Errorf("minimum %d%s", b.lo, b.unit)
	}
	if n > b.hi {
		return fmt.Errorf("maximum %d%s", b.hi, b.unit)
	}
	return nil
}

func (b intBounds) validate(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a number%s", b.unit)
	}
	return b.check(n)
}

var (
	cpuBounds       = intBounds{1, 128, " (vcpus)"}
	memoryBounds    = intBounds{1024, 1048576, " (in mb)"}
	osDiskBounds    = intBounds{20, 1000, " (in gb)"}
	dataDiskBounds  = intBounds{0, 5000, " (in gb)"}
	nodeCountBounds = intBounds{0, 100, " (nodes)"}
	vmidBounds      = intBounds{100, 999999999, ""}
	timeoutBounds   = intBounds{60, 86400, " (seconds)"}
)

// Preset field validators used by wizard input fields. Each wraps an
// intBounds range that the file-load validators also enforce.
var (
	ValidateCPU       = cpuBounds.validate
	ValidateMemory    = memoryBounds.validate
	ValidateOSDisk    = osDiskBounds.validate
	ValidateDataDisk  = dataDiskBounds.validate
	ValidateNodeCount = nodeCountBounds.validate
	ValidateVMID      = vmidBounds.validate
	ValidateTimeout   = timeoutBounds.validate
)

var terraformEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// ValidateTerraformEnv allows an empty string (runtime default applies) and
// otherwise requires an identifier valid as a directory name under
// infrastructure/terraform/environments/.
func ValidateTerraformEnv(value string) error {
	if value == "" {
		return nil
	}
	if !terraformEnvPattern.MatchString(value) {
		return errors.New("must start with a letter or underscore and contain only letters, digits, hyphens, or underscores")
	}
	return nil
}

// ValidateSSHFingerprint accepts an empty string (pin not set) or a value in
// the standard SHA256:<base64> format produced by ssh-keygen -lf / ssh-keyscan.
// This format is the only vocabulary accepted by sshpin.Verify.
func ValidateSSHFingerprint(value string) error {
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "SHA256:") || len(value) <= len("SHA256:") {
		return errors.New("must be in SHA256:<base64> format (from ssh-keygen -lf or ssh-keyscan | ssh-keygen -lf -)")
	}
	return nil
}

// ValidateBinDir accepts empty (default applies) or an absolute path
// without `..` elements. Relative paths resolve against an unpredictable
// cwd at install time; `..` is rejected before Clean resolves it because
// /usr/local/bin/../../etc would pass the absolute-path check yet land
// tool installs in /etc.
func ValidateBinDir(value string) error {
	if value == "" {
		return nil
	}
	if !filepath.IsAbs(value) {
		return errors.New("must be an absolute path (e.g. /usr/local/bin or /home/user/bin)")
	}
	if slices.Contains(strings.Split(value, string(filepath.Separator)), "..") {
		return errors.New("must not contain '..' path elements")
	}
	return nil
}
