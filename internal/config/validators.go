// Package config provides configuration management for the CLI.
package config

import (
	"bytes"
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

// ═══════════════════════════════════════════════════════════════════════════════
// REQUIRED FIELDS VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

type requiredFieldsValidator struct{}

func (v *requiredFieldsValidator) Scope() ValidationScope { return ScopeRequired }

func (v *requiredFieldsValidator) Validate(cfg *Config, result *ValidationResult) {
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

// ═══════════════════════════════════════════════════════════════════════════════
// ENUMS VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

type enumsValidator struct{}

func (v *enumsValidator) Scope() ValidationScope { return ScopeEnums }

func (v *enumsValidator) Validate(cfg *Config, result *ValidationResult) {
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

// ═══════════════════════════════════════════════════════════════════════════════
// NETWORKING VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

type networkingValidator struct{}

func (v *networkingValidator) Scope() ValidationScope { return ScopeNetworking }

func (v *networkingValidator) Validate(cfg *Config, result *ValidationResult) {
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

	if IsValidCIDR(podCIDR) && IsValidCIDR(serviceCIDR) && netutil.CIDRsOverlap(podCIDR, serviceCIDR) {
		result.AddError(FieldNetworkingPodCIDR, "overlaps with service CIDR")
	}

	if IsValidCIDR(podCIDR) && IsValidCIDR(machineCIDR) && netutil.CIDRsOverlap(podCIDR, machineCIDR) {
		result.AddError(FieldNetworkingPodCIDR, "overlaps with machine CIDR")
	}

	if IsValidCIDR(serviceCIDR) && IsValidCIDR(machineCIDR) && netutil.CIDRsOverlap(serviceCIDR, machineCIDR) {
		result.AddError(FieldNetworkingServiceCIDR, "overlaps with machine CIDR")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ADVANCED NETWORKING VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

type advancedNetworkingValidator struct{}

func (v *advancedNetworkingValidator) Scope() ValidationScope { return ScopeAdvancedNetworking }

func (v *advancedNetworkingValidator) Validate(cfg *Config, result *ValidationResult) {
	machineCIDR := cfg.Networking.MachineCIDR
	gateway := cfg.Networking.Gateway
	bastionIP := cfg.Networking.Bastion.IP
	staticIPStart := cfg.Networking.StaticIP.Start
	metallbPool := cfg.Networking.MetalLB.Pool

	if machineCIDR == "" || !IsValidCIDR(machineCIDR) {
		return
	}

	// ───────────────────────────────────────────────────────────────────────────
	// 1. CIDR Containment - All IPs must be within MachineCIDR
	// ───────────────────────────────────────────────────────────────────────────

	if gateway != "" && IsValidIP(gateway) && !netutil.IPInCIDR(gateway, machineCIDR) {
		result.AddError(FieldNetworkingGateway, fmt.Sprintf("must be within machine CIDR %s", machineCIDR))
	}

	if bastionIP != "" && IsValidIP(bastionIP) && !netutil.IPInCIDR(bastionIP, machineCIDR) {
		result.AddError(FieldNetworkingBastionIP, fmt.Sprintf("must be within machine CIDR %s", machineCIDR))
	}

	if staticIPStart != "" && IsValidIP(staticIPStart) && !netutil.IPInCIDR(staticIPStart, machineCIDR) {
		result.AddError(FieldNetworkingStaticIPStart, fmt.Sprintf("must be within machine CIDR %s", machineCIDR))
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

	// ───────────────────────────────────────────────────────────────────────────
	// 2. MetalLB Pool Validation
	// ───────────────────────────────────────────────────────────────────────────

	var metallbStart, metallbEnd string
	if metallbPool != "" {
		var err error
		metallbStart, metallbEnd, err = netutil.ParseIPPool(metallbPool)
		if err != nil {
			result.AddError(FieldNetworkingMetalLBPool, err.Error())
		} else {
			cmp, cmpErr := netutil.CompareIPs(metallbStart, metallbEnd)
			if cmpErr != nil {
				result.AddError(FieldNetworkingMetalLBPool, cmpErr.Error())
			} else if cmp > 0 {
				result.AddError(FieldNetworkingMetalLBPool, "start IP must be less than or equal to end IP")
			}

			if !netutil.IPInCIDR(metallbStart, machineCIDR) {
				result.AddError(FieldNetworkingMetalLBPool, fmt.Sprintf("start IP %s must be within machine CIDR %s", metallbStart, machineCIDR))
			}
			if !netutil.IPInCIDR(metallbEnd, machineCIDR) {
				result.AddError(FieldNetworkingMetalLBPool, fmt.Sprintf("end IP %s must be within machine CIDR %s", metallbEnd, machineCIDR))
			}
		}
	}

	// ───────────────────────────────────────────────────────────────────────────
	// 3. No Duplicate IPs - gateway and bastion must be unique
	// ───────────────────────────────────────────────────────────────────────────

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
				fmt.Sprintf("IP %s is already used by %s", nip.ip, prevField))
		} else {
			seen[normalized] = nip.name
		}
	}

	// ───────────────────────────────────────────────────────────────────────────
	// 4. StaticIP range must not overlap MetalLB pool
	// ───────────────────────────────────────────────────────────────────────────

	if staticIPStart != "" && IsValidIP(staticIPStart) && metallbStart != "" && metallbEnd != "" {
		totalNodes := 1 + cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
		staticIPEnd, err := netutil.CalculateVMIP(staticIPStart, totalNodes-1)
		if err == nil {
			if netutil.RangesOverlap(staticIPStart, staticIPEnd, metallbStart, metallbEnd) {
				result.AddError("networking.static_ip.start",
					fmt.Sprintf("range %s-%s overlaps with MetalLB pool %s", staticIPStart, staticIPEnd, metallbPool))
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RESOURCES VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

type resourcesValidator struct{}

func (v *resourcesValidator) Scope() ValidationScope { return ScopeResources }

func (v *resourcesValidator) Validate(cfg *Config, result *ValidationResult) {
	cp := cfg.Topology.ControlPlane

	if cp.CPU < MinCPUGeneric {
		result.AddError(FieldTopologyControlPlaneCPU, fmt.Sprintf("must have at least %d CPU", MinCPUGeneric))
	}

	minMemory := getMinMemoryForDistribution(cfg.Distribution.Type)
	if cp.Memory < minMemory {
		result.AddError(FieldTopologyControlPlaneMemory,
			fmt.Sprintf("must have at least %d MB of memory for %s", minMemory, cfg.Distribution.Type))
	}

	if cp.Disk < MinDiskGBGeneric {
		result.AddError(FieldTopologyControlPlaneDisk, fmt.Sprintf("must have at least %d GB of disk space", MinDiskGBGeneric))
	}

	w := cfg.Topology.Workers
	if w.Count > 0 {
		if w.CPU < MinCPUGeneric {
			result.AddError(FieldTopologyWorkersCPU, fmt.Sprintf("must have at least %d CPU", MinCPUGeneric))
		}
		if w.Memory < MinMemoryMBGeneric {
			result.AddError(FieldTopologyWorkersMemory, fmt.Sprintf("must have at least %d MB of memory", MinMemoryMBGeneric))
		}
		if w.Disk < MinDiskGBGeneric {
			result.AddError(FieldTopologyWorkersDisk, fmt.Sprintf("must have at least %d GB of disk space", MinDiskGBGeneric))
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PROVIDER VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

type providerValidator struct{}

func (v *providerValidator) Scope() ValidationScope { return ScopeProvider }

func (v *providerValidator) Validate(cfg *Config, result *ValidationResult) {
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
}

// ═══════════════════════════════════════════════════════════════════════════════
// ADDONS VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

// addonsValidator checks structural correctness; addon-specific validation
// is delegated to each addon's ValidateSettings at install time.
type addonsValidator struct{}

func (v *addonsValidator) Scope() ValidationScope { return ScopeFeatures }

func (v *addonsValidator) Validate(cfg *Config, result *ValidationResult) {
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

// ═══════════════════════════════════════════════════════════════════════════════
// HTTP SERVER VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

type httpServerValidator struct{}

func (v *httpServerValidator) Scope() ValidationScope { return ScopeHTTPServer }

func (v *httpServerValidator) Validate(cfg *Config, result *ValidationResult) {
	if cfg.HTTPServer.Port != 0 {
		if cfg.HTTPServer.Port < 1 || cfg.HTTPServer.Port > 65535 {
			result.AddError(FieldHTTPServerPort, "must be a valid port number (1-65535)")
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// DISTRIBUTION VALIDATOR (OKD)
// ═══════════════════════════════════════════════════════════════════════════════

type distributionValidator struct{}

func (v *distributionValidator) Scope() ValidationScope { return ScopeDistribution }

func (v *distributionValidator) Validate(cfg *Config, result *ValidationResult) {
	if cfg.Distribution.Type == DistributionOKD {
		ValidateOKDConfig(cfg, result)
	}
}

// ValidateOKDVersion validates an OKD version string format.
func ValidateOKDVersion(version string) error {
	if version == "" {
		return fmt.Errorf("okd version is required")
	}

	if !okdVersionPattern.MatchString(version) {
		return fmt.Errorf("okd version must be in format X.Y.Z-okd-<suffix> (e.g., 4.18.0-okd-scos.10)")
	}

	return nil
}

// ValidateHAMasters validates that master count is odd for proper etcd quorum.
func ValidateHAMasters(count int) error {
	if count > 1 && count%2 == 0 {
		return fmt.Errorf("master replicas should be odd for HA quorum (1, 3, or 5), got %d", count)
	}
	return nil
}

// ValidateOKDConfig validates OKD-specific configuration requirements.
func ValidateOKDConfig(cfg *Config, result *ValidationResult) {
	if cfg.Distribution.Type == DistributionOKD {
		if err := ValidateOKDVersion(cfg.Distribution.Version); err != nil {
			result.AddError(FieldDistributionVersion, err.Error())
		}

		if cfg.Topology.ControlPlane.Memory < MinMemoryMBControlPlaneOKD {
			result.AddError(FieldTopologyControlPlaneMemory,
				fmt.Sprintf("OKD requires at least %d MB (%d GB) of memory for control plane nodes", MinMemoryMBControlPlaneOKD, MinMemoryMBControlPlaneOKD/1024))
		}

		if cfg.Topology.ControlPlane.CPU < MinCPUControlPlaneOKD {
			result.AddError(FieldTopologyControlPlaneCPU,
				fmt.Sprintf("OKD requires at least %d vCPUs for control plane nodes", MinCPUControlPlaneOKD))
		}

		if cfg.Topology.ControlPlane.Disk < MinDiskGBControlPlaneOKD {
			result.AddError(FieldTopologyControlPlaneDisk,
				fmt.Sprintf("OKD requires at least %d GB of disk space for control plane nodes", MinDiskGBControlPlaneOKD))
		}

		if err := ValidateHAMasters(cfg.Topology.ControlPlane.Count); err != nil {
			result.AddError(FieldTopologyControlPlaneCount, err.Error())
		}

		if cfg.Topology.Workers.Count > 0 {
			if cfg.Topology.Workers.Memory < MinMemoryMBWorkerOKD {
				result.AddError(FieldTopologyWorkersMemory,
					fmt.Sprintf("OKD workers require at least %d MB (%d GB) of memory", MinMemoryMBWorkerOKD, MinMemoryMBWorkerOKD/1024))
			}
			if cfg.Topology.Workers.CPU < MinCPUWorkerOKD {
				result.AddError(FieldTopologyWorkersCPU,
					fmt.Sprintf("OKD workers require at least %d vCPUs", MinCPUWorkerOKD))
			}
			if cfg.Topology.Workers.Disk < MinDiskGBWorkerOKD {
				result.AddError(FieldTopologyWorkersDisk,
					fmt.Sprintf("OKD workers require at least %d GB of disk space", MinDiskGBWorkerOKD))
			}
		}
	}
}

// ValidateWithOKD performs full validation including OKD-specific checks.
func ValidateWithOKD(cfg *Config) *ValidationResult {
	result := cfg.Validate()
	if cfg.Distribution.Type == DistributionOKD {
		ValidateOKDConfig(cfg, result)
	}

	return result
}

// ═══════════════════════════════════════════════════════════════════════════════
// FILES VALIDATOR
// ═══════════════════════════════════════════════════════════════════════════════

type filesValidator struct{}

func (v *filesValidator) Scope() ValidationScope { return ScopeFiles }

func (v *filesValidator) Validate(cfg *Config, result *ValidationResult) {
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

// ═══════════════════════════════════════════════════════════════════════════════
// VALIDATION HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════════

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

// IsValidIP checks if a string is a valid IP address.
func IsValidIP(s string) bool {
	return net.ParseIP(s) != nil
}

// IsValidCIDR checks if a string is a valid CIDR notation.
func IsValidCIDR(s string) bool {
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// isValidNetmask checks if a string is a valid netmask (dotted-quad or CIDR prefix).
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

// CIDROverlaps checks if two CIDR ranges overlap.
func CIDROverlaps(cidr1, cidr2 string) bool {
	_, net1, err1 := net.ParseCIDR(cidr1)
	_, net2, err2 := net.ParseCIDR(cidr2)
	if err1 != nil || err2 != nil {
		return false
	}
	return net1.Contains(net2.IP) || net2.Contains(net1.IP) ||
		net2.Contains(cidrLastIP(net1)) || net1.Contains(cidrLastIP(net2))
}

// cidrLastIP returns the last IP address in a CIDR range.
func cidrLastIP(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP))
	for i := range ip {
		ip[i] = n.IP[i] | ^n.Mask[i]
	}
	return ip
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
		DistributionOKD: MinMemoryMBControlPlaneOKD, // OKD requires significant resources
	}
	if mem, ok := minMemory[d]; ok {
		return mem
	}
	return DefaultMinMemoryMB
}

// ═══════════════════════════════════════════════════════════════════════════════
// COMMON VALIDATORS (for CLI and TUI)
// ═══════════════════════════════════════════════════════════════════════════════

// ValidateIP validates that a string is a valid IP address.
func ValidateIP(value string) error {
	if !IsValidIP(value) {
		return errors.New("invalid ip address")
	}
	return nil
}

// ValidateCIDR validates that a string is valid CIDR notation.
func ValidateCIDR(value string) error {
	if !IsValidCIDR(value) {
		return errors.New("invalid cidr format (e.g., 192.168.1.0/24)")
	}
	return nil
}

// ValidateIntRange creates a validator for integer values within a range.
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

// ValidatePortNumber validates that a port number is in range 1-65535.
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

// ValidateIPRange validates an IP range in "start-end" format.
func ValidateIPRange(value string) error {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return errors.New("invalid range format (e.g., 192.168.1.200-192.168.1.230)")
	}

	startIPStr := strings.TrimSpace(parts[0])
	endIPStr := strings.TrimSpace(parts[1])

	startIP := net.ParseIP(startIPStr)
	if startIP == nil {
		return fmt.Errorf("invalid start ip: %s", startIPStr)
	}
	endIP := net.ParseIP(endIPStr)
	if endIP == nil {
		return fmt.Errorf("invalid end ip: %s", endIPStr)
	}

	if bytes.Compare(startIP.To16(), endIP.To16()) > 0 {
		return errors.New("start ip must be less than or equal to end ip")
	}

	return nil
}

// ValidateHostPrefix validates the host prefix is in range 20-28.
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
