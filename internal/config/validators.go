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

	// env becomes root-privileged terraform's cwd, so "../" must fail closed
	// here; validateTerraformEnvDir checks dir existence separately.
	if env := cfg.Deployment.TerraformEnv; env != "" {
		if err := ValidateTerraformEnv(env); err != nil {
			result.AddError(FieldDeploymentTerraformEnv, err.Error())
		}
	}
}

// validateTerraformEnvDir requires a matching directory under projectRoot for
// custom environments; "production" is trusted without a disk check.
func validateTerraformEnvDir(cfg *Config, projectRoot string, result *ValidationResult) {
	env := cfg.Deployment.TerraformEnv
	// A pattern-invalid env already failed validateEnums; avoid a duplicate error here.
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

	// bastion.ip/static_ip.dns feed the kernel cmdline verbatim
	// (nameserver=%s via --live-karg-append); a non-IP value injects extra kargs.
	if cfg.Networking.Bastion.IP != "" && !IsValidIP(cfg.Networking.Bastion.IP) {
		result.AddError(FieldNetworkingBastionIP, "must be a valid IP address")
	}
	if cfg.Networking.StaticIP.DNS != "" && !IsValidIP(cfg.Networking.StaticIP.DNS) {
		result.AddError(FieldNetworkingStaticIPDNS, "must be a valid IP address")
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

	// The bastion runs both dnsmasq (provision/kargs.go) and the ignition
	// server (provision/apache.go); a mismatch here fails deep in setup
	// with no earlier diagnostic.
	if bastionIP != "" {
		if dns := cfg.Networking.StaticIP.DNS; dns != "" && dns != bastionIP {
			result.AddError(FieldNetworkingStaticIPDNS, "must match networking.bastion.ip — dnsmasq (the VMs' DNS server) runs on the bastion")
		}
		if ignitionIP := cfg.HTTPServer.IgnitionServerIP; ignitionIP != "" && ignitionIP != bastionIP {
			result.AddError(FieldHTTPServerIP, "must match networking.bastion.ip — the ignition http server binds to the bastion")
		}
	}

	// Parsed-address comparison catches textual variants of the same IP.
	if gwAddr, err := netip.ParseAddr(gateway); err == nil {
		if bAddr, err := netip.ParseAddr(bastionIP); err == nil && bAddr == gwAddr {
			result.AddError(FieldNetworkingBastionIP,
				fmt.Sprintf("ip %s is already used by gateway", bastionIP))
		}
	}
}

// validateStaticIPCollisions rejects static_ip.start colliding with the
// proxmox host or ignition server IP — the bootstrap VM would ARP-fight it.
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
	host := stripProxmoxScheme(proxmox.Host)
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	return host
}

// stripProxmoxScheme drops a leading http(s):// — trimming only http:// let
// https://host:port reach SplitHostPort and mis-split on the second colon.
func stripProxmoxScheme(host string) string {
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	return host
}

// validateHostPort accepts a bare hostname, IP, or host:port; rejecting '/'
// and '@' fails closed on scheme/userinfo input SplitHostPort would mis-split
// (e.g. "https://h" → host "https").
func validateHostPort(value string) error {
	if value == "" || strings.ContainsAny(value, "/@") {
		return errors.New("must be a valid hostname or IP address")
	}
	host := value
	if strings.Contains(value, ":") {
		h, _, err := net.SplitHostPort(value)
		if err != nil {
			return errors.New("must be a valid host or host:port")
		}
		host = h
	}
	if host == "" || !isValidHostOrIP(host) {
		return errors.New("must be a valid hostname or IP address")
	}
	return nil
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

	validateBootstrap(cfg, result)
}

// validateBootstrap enforces that topology.bootstrap models the single
// ephemeral pivot VM: Count must be 1, and DiskGB, if set, must match the
// control-plane OS disk terraform actually uses.
func validateBootstrap(cfg *Config, result *ValidationResult) {
	b := cfg.Topology.Bootstrap
	if b.Count != 0 && b.Count != 1 {
		result.AddError(FieldTopologyBootstrapCount,
			"the bootstrap node is a single ephemeral pivot vm; count must be 1 (or omit the field)")
	}
	effectiveOSDisk := cfg.Topology.ControlPlane.DiskGB
	if effectiveOSDisk == 0 {
		effectiveOSDisk = DefaultOSDiskGB
	}
	if b.DiskGB != 0 && b.DiskGB != effectiveOSDisk {
		result.AddError(FieldTopologyBootstrapDisk,
			fmt.Sprintf("has no effect: the bootstrap vm always uses the control-plane os disk size (%d gb); remove the field or match %s", effectiveOSDisk, FieldTopologyControlPlaneDisk))
	}
}

// checkNodeResourceCaps applies the wizard's upper bounds; floors belong to checkNodeResources.
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

// validateDeployment mirrors the wizard's advanced-step constraints: zero
// timeouts fall back to compiled defaults, and BinDir is checked post
// ~-expansion so a value ResolveBinDir would silently discard fails here.
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

// validateVMIDBase bounds topology.vm_id_base (same range as the wizard's
// ValidateVMID) so an out-of-range base fails here, not inside terraform
// apply; the overflow check also keeps the highest computed vmid under the ceiling.
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

// validatePlacementCounts rejects a placement list longer than the group's
// topology count — terraform silently ignores the excess by index. Shorter
// lists are valid; unassigned VMs fall back to provider.proxmox.node.
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
		if err := validateHostPort(stripProxmoxScheme(proxmox.Host)); err != nil {
			result.AddError(FieldProxmoxHost, err.Error())
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

// additionalNetworkModels allowlists provider.proxmox.additional_networks[].model;
// empty selects virtio at render time.
var additionalNetworkModels = []string{"virtio", "e1000", "rtl8139", "vmxnet3"}

// validateAdditionalNetworks mirrors Bridge validation per NIC: Bridge/Model
// render into terraform.tfvars HCL where %q doesn't neutralize ${…}
// interpolation, so these charset gates are the injection boundary.
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

// httpRootPattern allowlists DocumentRoot characters; the value is
// interpolated raw into a root-owned Apache vhost, so unknown metacharacters
// must fail closed.
var httpRootPattern = regexp.MustCompile(`^/[A-Za-z0-9._/-]*$`)

func validateHTTPServer(cfg *Config, result *ValidationResult) {
	if cfg.HTTPServer.IgnitionServerIP != "" && !IsValidIP(cfg.HTTPServer.IgnitionServerIP) {
		result.AddError(FieldHTTPServerIP, "must be a valid IPv4 or IPv6 literal")
	}

	if cfg.HTTPServer.Root != "" {
		switch {
		case !filepath.IsAbs(cfg.HTTPServer.Root):
			result.AddError(FieldHTTPServerRoot, "must be an absolute path")
		case !httpRootPattern.MatchString(cfg.HTTPServer.Root):
			result.AddError(FieldHTTPServerRoot, "must contain only letters, digits, and ._-/ (no shell or Apache directive metacharacters)")
		case slices.Contains(strings.Split(cfg.HTTPServer.Root, "/"), ".."):
			// The allowlist admits '.' and '/', so ".." slips past it; reject traversal separately.
			result.AddError(FieldHTTPServerRoot, "must not contain a '..' path segment")
		}
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

// ValidateOKDConfig applies OKD version and resource floors when cfg is OKD;
// otherwise it is a no-op.
func ValidateOKDConfig(cfg *Config, result *ValidationResult) {
	if cfg.Distribution.Type != DistributionOKD {
		return
	}

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
	// Reject 0.0.0.0 — legal bit pattern but meaningless as a host netmask.
	if mask == 0 {
		return false
	}
	// A canonical netmask is N ones then zeros; ^mask+1 isolates the lowest
	// zero-bit, so a contiguous mask is a power of two in ^mask+1 (all-ones
	// is the special case where ~m is zero).
	inverted := ^mask
	return inverted == 0 || (inverted&(inverted+1)) == 0
}

// Listed rather than compared inline so a second variant lands in one place.
func supportedDistributions() []DistributionType {
	return []DistributionType{
		DistributionOKD,
	}
}

func supportedProviders() []ProviderType {
	return []ProviderType{
		ProviderProxmox,
	}
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

// ValidateProxmoxNodeName rejects names not matching [a-zA-Z][a-zA-Z0-9_-]*,
// the same pattern enforced by validateProxmoxConfig and the pveshRun guard.
func ValidateProxmoxNodeName(value string) error {
	if !proxmoxNamePattern.MatchString(value) {
		return errors.New("must start with a letter and contain only alphanumeric, hyphens, or underscores")
	}
	return nil
}

// ValidateProxmoxHost accepts a bare hostname, IP, or host:port, and rejects
// scheme/userinfo-prefixed values (https://host, user:pass@host).
func ValidateProxmoxHost(value string) error {
	return validateHostPort(value)
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

// ValidateGatewayInCIDR reports an error if gateway is not inside cidr; it
// returns nil when either value is missing or malformed.
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

// intBounds is the single [lo, hi] range shared by wizard and file-load
// validators so the two surfaces can't drift.
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

// Preset field validators for wizard input fields; each wraps an intBounds range.
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

// ValidateBinDir accepts empty or an absolute path without `..` elements.
// `..` is rejected before Clean resolves it because /usr/local/bin/../../etc
// would pass the absolute-path check yet land installs in /etc.
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
