package config

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

var (
	dnsLabelPattern   = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	domainPattern     = regexp.MustCompile(`^([a-z0-9]([-a-z0-9]*[a-z0-9])?\.)*[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	okdVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-okd-[a-zA-Z0-9.-]+$`)

	interfaceNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*$`)
	proxmoxNamePattern   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
)

func validateRequired(cfg *Config, result *ValidationResult) {
	if cfg.Cluster.Name == "" {
		result.AddError(FieldClusterName, "cluster name is required")
	}
	if cfg.Cluster.Domain == "" {
		result.AddError(FieldClusterDomain, "cluster domain is required")
	}
	if cfg.Distribution.Type == "" {
		result.AddError(FieldDistributionType, "distribution type is required")
	}
	if cfg.Distribution.Version == "" {
		result.AddError(FieldDistributionVersion, "distribution version is required")
	}
	if cfg.Provider.Type == "" {
		result.AddError(FieldProviderType, "provider type is required")
	}
	if cfg.Topology.ControlPlane.Count < 1 {
		result.AddError(FieldTopologyControlPlaneCount, "must have at least 1 control plane node")
	}
	if cfg.Networking.MachineCIDR == "" {
		result.AddError(FieldNetworkingMachineCIDR, "machine CIDR is required")
	}
	if cfg.Networking.PodCIDR == "" {
		result.AddError(FieldNetworkingPodCIDR, "pod CIDR is required")
	}
	if cfg.Networking.ServiceCIDR == "" {
		result.AddError(FieldNetworkingServiceCIDR, "service CIDR is required")
	}
	if cfg.Networking.Gateway == "" {
		result.AddError(FieldNetworkingGateway, "gateway is required")
	}
	if len(cfg.Networking.DNS) == 0 {
		result.AddError(FieldNetworkingDNS, "at least one DNS server is required")
	}
}

func validateEnums(cfg *Config, result *ValidationResult) {
	if cfg.Cluster.Name != "" {
		if !IsValidDNSLabel(cfg.Cluster.Name) {
			result.AddError(FieldClusterName, "must be a valid DNS label (lowercase, alphanumeric, hyphens)")
		} else if len(cfg.Cluster.Name) > 63 {
			result.AddError(FieldClusterName, "must be 63 characters or less")
		}
	}

	if cfg.Cluster.Domain != "" && !isValidDomain(cfg.Cluster.Domain) {
		result.AddError(FieldClusterDomain, "must be a valid domain name")
	}

	if cfg.Distribution.Type != "" && !isValidDistribution(cfg.Distribution.Type) {
		result.AddError(FieldDistributionType, fmt.Sprintf("unsupported distribution: %s", cfg.Distribution.Type))
	}

	if cfg.Provider.Type != "" && !isValidProvider(cfg.Provider.Type) {
		result.AddError(FieldProviderType, fmt.Sprintf("unsupported provider: %s", cfg.Provider.Type))
	}
}

// checkCIDROverlap appends a validation error if the two CIDRs overlap.
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

// checkIPInCIDR appends a validation error if ip is not within cidr.
func checkIPInCIDR(ip, cidr, field, cidrName string, result *ValidationResult) {
	if ip == "" || !IsValidIP(ip) || !IsValidCIDR(cidr) {
		return
	}
	ok, err := netutil.IPInCIDR(ip, cidr)
	if err != nil {
		result.AddError(field, fmt.Sprintf("cannot check CIDR membership: %v", err))
	} else if !ok {
		result.AddError(field, fmt.Sprintf("must be within %s %s", cidrName, cidr))
	}
}

// checkNodeResources validates CPU/memory/disk minimums for a node config.
func checkNodeResources(node NodeConfig, minCPU, minMemory, minDisk int, cpuField, memField, diskField, label string, result *ValidationResult) {
	if node.CPU < minCPU {
		result.AddError(cpuField, fmt.Sprintf("must have at least %d vCPUs for %s", minCPU, label))
	}
	if node.Memory < minMemory {
		result.AddError(memField, fmt.Sprintf("must have at least %d MB of memory for %s", minMemory, label))
	}
	if node.Disk < minDisk {
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

	if machineCIDR == "" || !IsValidCIDR(machineCIDR) {
		return
	}

	checkIPInCIDR(gateway, machineCIDR, FieldNetworkingGateway, "machine CIDR", result)
	checkIPInCIDR(bastionIP, machineCIDR, FieldNetworkingBastionIP, "machine CIDR", result)
	checkIPInCIDR(staticIPStart, machineCIDR, FieldNetworkingStaticIPStart, "machine CIDR", result)

	if cfg.Networking.Bastion.VIP != "" {
		if !IsValidIP(cfg.Networking.Bastion.VIP) {
			result.AddError("networking.bastion.vip", "must be a valid IP address")
		} else {
			checkIPInCIDR(cfg.Networking.Bastion.VIP, machineCIDR, "networking.bastion.vip", "machine CIDR", result)
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
		if netmask == "" {
			result.AddError(FieldNetworkingStaticIPNetmask, "netmask is required when using static IPs")
		} else if !isValidNetmask(netmask) {
			result.AddError(FieldNetworkingStaticIPNetmask, "must be a valid netmask (e.g., 255.255.255.0 or /24)")
		}

		iface := cfg.Networking.StaticIP.Interface
		if iface == "" {
			result.AddError(FieldNetworkingStaticIPIface, "interface name is required when using static IPs")
		} else if !interfaceNamePattern.MatchString(iface) {
			result.AddError(FieldNetworkingStaticIPIface, "must be a valid interface name (e.g., eth0, ens18)")
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

	seen := make(map[string]string)
	for _, nip := range uniqueIPs {
		if nip.ip == "" || !IsValidIP(nip.ip) {
			continue
		}
		parsed := net.ParseIP(nip.ip)
		normalized := parsed.String()

		if prevField, exists := seen[normalized]; exists {
			result.AddError(fmt.Sprintf("networking.%s", nip.name),
				fmt.Sprintf("ip %s is already used by %s", nip.ip, prevField))
		} else {
			seen[normalized] = nip.name
		}
	}

}

func validateResources(cfg *Config, result *ValidationResult) {
	minMemory := getMinMemoryForDistribution(cfg.Distribution.Type)
	checkNodeResources(cfg.Topology.ControlPlane, MinCPUGeneric, minMemory, MinDiskGBGeneric,
		FieldTopologyControlPlaneCPU, FieldTopologyControlPlaneMemory, FieldTopologyControlPlaneDisk,
		string(cfg.Distribution.Type), result)

	if cfg.Topology.Workers.Count > 0 {
		checkNodeResources(cfg.Topology.Workers, MinCPUGeneric, MinMemoryMBGeneric, MinDiskGBGeneric,
			FieldTopologyWorkersCPU, FieldTopologyWorkersMemory, FieldTopologyWorkersDisk,
			"workers", result)
	}
}

func validateProvider(cfg *Config, result *ValidationResult) {
	switch cfg.Provider.Type {
	case ProviderProxmox:
		validateProxmoxConfig(cfg.Provider.Proxmox, result)
	}
}

func validateProxmoxConfig(proxmox *ProxmoxConfig, result *ValidationResult) {
	if proxmox == nil {
		result.AddError(FieldProviderProxmox, "proxmox configuration is required when using proxmox provider")
		return
	}

	if proxmox.Host == "" {
		result.AddError(FieldProxmoxHost, "proxmox host is required")
	} else {
		host := proxmox.Host
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

	for i, node := range proxmox.MasterNodes {
		if node != "" && !proxmoxNamePattern.MatchString(node) {
			result.AddError(fmt.Sprintf("proxmox.master_nodes[%d]", i), "must be a valid Proxmox node name")
		}
	}
	for i, node := range proxmox.WorkerNodes {
		if node != "" && !proxmoxNamePattern.MatchString(node) {
			result.AddError(fmt.Sprintf("proxmox.worker_nodes[%d]", i), "must be a valid Proxmox node name")
		}
	}
}

func validateAddons(cfg *Config, result *ValidationResult) {
	if cfg.Addons == nil {
		return
	}

	for name, ac := range cfg.Addons {
		if !ac.Enabled {
			continue
		}
		_ = name // addon-specific validation is handled by the addon registry at install time
	}
}

func validateHTTPServer(cfg *Config, result *ValidationResult) {
	if cfg.HTTPServer.Port != 0 {
		if cfg.HTTPServer.Port < 1 || cfg.HTTPServer.Port > 65535 {
			result.AddError(FieldHTTPServerPort, "must be a valid port number (1-65535)")
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
		return fmt.Errorf("okd version must be in format X.Y.Z-okd-<suffix> (e.g., 4.18.0-okd-scos.10)")
	}

	return nil
}

// ValidateHAMasters validates that master count is odd for proper etcd quorum.
func validateHAMasters(count int) error {
	if count > 1 && count%2 == 0 {
		return fmt.Errorf("master replicas should be odd for ha quorum (1, 3, or 5), got %d", count)
	}
	return nil
}

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

// IsValidDNSLabel checks if a string is a valid DNS label (RFC 1123).
func IsValidDNSLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	return dnsLabelPattern.MatchString(s)
}

func isValidDomain(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	return domainPattern.MatchString(s)
}

func isValidHostOrIP(s string) bool {
	return IsValidIP(s) || isValidDomain(s)
}

func IsValidIP(s string) bool {
	return net.ParseIP(s) != nil
}

func IsValidCIDR(s string) bool {
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

func isValidNetmask(s string) bool {
	prefix := strings.TrimPrefix(s, "/")
	if n, err := strconv.Atoi(prefix); err == nil && n >= 0 && n <= 32 {
		return true
	}

	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	// Valid netmask: contiguous 1s followed by contiguous 0s
	mask := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
	inverted := ^mask
	return inverted == 0 || (inverted&(inverted+1)) == 0
}

func isValidDistribution(d DistributionType) bool {
	for _, dist := range SupportedDistributions() {
		if dist == d {
			return true
		}
	}
	return false
}

func isValidProvider(p ProviderType) bool {
	for _, prov := range SupportedProviders() {
		if prov == p {
			return true
		}
	}
	return false
}

func getMinMemoryForDistribution(d DistributionType) int {
	minMemory := map[DistributionType]int{
		DistributionOKD: MinMemoryMBControlPlaneOKD,
	}
	if mem, ok := minMemory[d]; ok {
		return mem
	}
	return DefaultMinMemoryMB
}

func ValidateIP(value string) error {
	if !IsValidIP(value) {
		return errors.New("invalid ip address")
	}
	return nil
}

func ValidateCIDR(value string) error {
	if !IsValidCIDR(value) {
		return errors.New("invalid cidr format (e.g., 192.168.1.0/24)")
	}
	return nil
}

func ValidateIntRange(unit string, min, max int) func(string) error {
	return func(value string) error {
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("must be a number%s", unit)
		}
		if n < min {
			return fmt.Errorf("minimum %d%s", min, unit)
		}
		if n > max {
			return fmt.Errorf("maximum %d%s", max, unit)
		}
		return nil
	}
}

func ValidatePortNumber(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil {
		return errors.New("must be a valid number")
	}
	if port < 1 || port > 65535 {
		return errors.New("must be between 1 and 65535")
	}
	return nil
}

func ValidateHostPrefix(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return errors.New("must be a number")
	}
	if n < 20 || n > 28 {
		return fmt.Errorf("must be between 20-28 (got %d)", n)
	}
	return nil
}

var (
	ValidateCPU       = ValidateIntRange(" (vcpus)", 1, 128)
	ValidateMemory    = ValidateIntRange(" (in mb)", 1024, 1048576)
	ValidateOSDisk    = ValidateIntRange(" (in gb)", 20, 1000)
	ValidateDataDisk  = ValidateIntRange(" (in gb)", 50, 5000)
	ValidateNodeCount = ValidateIntRange(" (nodes)", 1, 100)
	ValidateVMID      = ValidateIntRange("", 100, 999999999)
	ValidateTimeout   = ValidateIntRange(" (seconds)", 60, 86400)
)
