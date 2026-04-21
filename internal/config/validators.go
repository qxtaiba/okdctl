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
)

var (
	dnsLabelPattern   = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	domainPattern     = regexp.MustCompile(`^([a-z0-9]([-a-z0-9]*[a-z0-9])?\.)*[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	okdVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+-okd-[a-zA-Z0-9.-]+$`)

	interfaceNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*$`)
	proxmoxNamePattern   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
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
	if cfg.Cluster.Name != "" && !IsValidDNSLabel(cfg.Cluster.Name) {
		result.AddError(FieldClusterName, "must be a valid DNS label (lowercase, alphanumeric, hyphens)")
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
	if cfg.Provider.Type == ProviderProxmox {
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
	return slices.Contains(SupportedDistributions(), d)
}

func isValidProvider(p ProviderType) bool {
	return slices.Contains(SupportedProviders(), p)
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

// ValidateClusterName returns a descriptive error if value violates the
// DNS-1123 cluster-name grammar (2-63 chars, lowercase a-z/0-9/-, must
// start with a letter).
func ValidateClusterName(value string) error {
	if len(value) < 2 {
		return errors.New("must be at least 2 characters")
	}
	if !IsValidDNSLabel(value) {
		return errors.New("must start with letter, contain only lowercase letters, numbers, hyphens, max 63 chars")
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

// ValidateIntRange returns a validator that requires value to parse as an
// integer in [lo, hi]. unit is appended to error messages for context.
func ValidateIntRange(unit string, lo, hi int) func(string) error {
	return func(value string) error {
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("must be a number%s", unit)
		}
		if n < lo {
			return fmt.Errorf("minimum %d%s", lo, unit)
		}
		if n > hi {
			return fmt.Errorf("maximum %d%s", hi, unit)
		}
		return nil
	}
}

// ValidatePortNumber requires value to parse as a port in [1, 65535].
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

// Preset field validators used by wizard input fields. Each wraps
// ValidateIntRange with the appropriate unit label and bounds.
var (
	ValidateCPU       = ValidateIntRange(" (vcpus)", 1, 128)
	ValidateMemory    = ValidateIntRange(" (in mb)", 1024, 1048576)
	ValidateOSDisk    = ValidateIntRange(" (in gb)", 20, 1000)
	ValidateNodeCount = ValidateIntRange(" (nodes)", 1, 100)
	ValidateVMID      = ValidateIntRange("", 100, 999999999)
	ValidateTimeout   = ValidateIntRange(" (seconds)", 60, 86400)
)

var terraformEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// ValidateTerraformEnv allows an empty string (runtime default applies) and
// otherwise requires a terraform-workspace-shaped identifier.
func ValidateTerraformEnv(value string) error {
	if value == "" {
		return nil
	}
	if !terraformEnvPattern.MatchString(value) {
		return errors.New("must start with a letter or underscore and contain only letters, digits, hyphens, or underscores")
	}
	return nil
}

// ValidateBinDir accepts empty (default applies) or an absolute path.
// Relative paths resolve against an unpredictable cwd at install time.
func ValidateBinDir(value string) error {
	if value == "" {
		return nil
	}
	if !filepath.IsAbs(value) {
		return errors.New("must be an absolute path (e.g. /usr/local/bin or /home/user/bin)")
	}
	return nil
}
