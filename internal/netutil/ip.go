package netutil

import (
	"fmt"
	"net/netip"
)

// DefaultVIPLastOctet is the final IPv4 octet used when deriving a VIP from
// a static-IP range without an explicit override.
const DefaultVIPLastOctet = 10

// DefaultNetmask is the /24 subnet mask applied when no explicit netmask is
// configured — matches the typical homelab /24 and the Proxmox default bridge.
const DefaultNetmask = "255.255.255.0"

// CIDRToNetmask converts an IPv4 CIDR like "192.168.1.0/24" to its dotted
// netmask form "255.255.255.0" as consumed by HAProxy and dnsmasq templates.
// IPv6 CIDRs are rejected because downstream templates are IPv4-only.
func CIDRToNetmask(cidr string) (string, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return "", fmt.Errorf("IPv6 CIDR not supported: %q", cidr)
	}
	bits := prefix.Bits()
	var mask uint32
	if bits > 0 {
		mask = ^uint32(0) << (32 - bits)
	}
	return fmt.Sprintf("%d.%d.%d.%d", byte(mask>>24), byte(mask>>16), byte(mask>>8), byte(mask)), nil
}

// ValidateIPRangeInCIDR checks that startIP and the next count-1 addresses
// all fall inside cidr. Only IPv4 is supported.
func ValidateIPRangeInCIDR(startIP string, count int, cidr string) error {
	if count <= 0 {
		return fmt.Errorf("count must be positive: %d", count)
	}

	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	start, err := netip.ParseAddr(startIP)
	if err != nil {
		return fmt.Errorf("invalid IPv4 address %q: %w", startIP, err)
	}
	if !start.Is4() {
		return fmt.Errorf("IPv6 not supported: %q", startIP)
	}

	if !prefix.Contains(start) {
		return fmt.Errorf("start IP %s is not within CIDR %s", startIP, cidr)
	}

	endIP, err := CalculateVMIP(startIP, count-1)
	if err != nil {
		return fmt.Errorf("failed to calculate end of range: %w", err)
	}

	end, _ := netip.ParseAddr(endIP)
	if !prefix.Contains(end) {
		return fmt.Errorf("IP range %s + %d addresses exceeds CIDR %s (last IP would be %s)", startIP, count, cidr, endIP)
	}

	return nil
}

// CalculateVMIP returns the IPv4 address obtained by adding index to startIP.
// Negative index is rejected, and 32-bit overflow is guarded.
func CalculateVMIP(startIP string, index int) (string, error) {
	if index < 0 {
		return "", fmt.Errorf("index cannot be negative: %d", index)
	}

	addr, err := netip.ParseAddr(startIP)
	if err != nil {
		return "", fmt.Errorf("invalid IPv4 address %q: %w", startIP, err)
	}
	if !addr.Is4() {
		return "", fmt.Errorf("IPv6 not supported: %q", startIP)
	}

	raw := addr.As4()
	ipInt := uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])

	if uint64(ipInt)+uint64(index) > uint64(^uint32(0)) {
		return "", fmt.Errorf("IP calculation would overflow: %s + %d", startIP, index)
	}

	newInt := ipInt + uint32(index) //nolint:gosec // G115: overflow checked above via uint64 guard
	result := netip.AddrFrom4([4]byte{
		byte(newInt >> 24), byte(newInt >> 16), byte(newInt >> 8), byte(newInt), //nolint:gosec // G115: byte conversion of right-shifted uint32 is safe
	})
	return result.String(), nil
}

// ResolveVIP returns explicitVIP if set (after validation), otherwise falls
// back to DeriveVIPFromStaticIP which uses the .10 last octet convention.
func ResolveVIP(explicitVIP, staticIPStart string) (string, error) {
	if explicitVIP != "" {
		addr, err := netip.ParseAddr(explicitVIP)
		if err != nil {
			return "", fmt.Errorf("invalid VIP address %q: %w", explicitVIP, err)
		}
		if !addr.Is4() {
			return "", fmt.Errorf("IPv6 VIP not supported: %q", explicitVIP)
		}
		return addr.String(), nil
	}
	return DeriveVIPFromStaticIP(staticIPStart)
}

// DeriveVIPFromStaticIP replaces the last octet of staticIPStart with
// DefaultVIPLastOctet to yield a conventional VIP in the same /24.
func DeriveVIPFromStaticIP(staticIPStart string) (string, error) {
	addr, err := netip.ParseAddr(staticIPStart)
	if err != nil {
		return "", fmt.Errorf("invalid IPv4 address %q: %w", staticIPStart, err)
	}
	if !addr.Is4() {
		return "", fmt.Errorf("IPv6 not supported: %q", staticIPStart)
	}
	octets := addr.As4()
	return fmt.Sprintf("%d.%d.%d.%d", octets[0], octets[1], octets[2], DefaultVIPLastOctet), nil
}

// IPInCIDR reports whether ip is contained within cidr.
func IPInCIDR(ip, cidr string) (bool, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false, fmt.Errorf("invalid IP address %q: %w", ip, err)
	}

	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	return prefix.Contains(addr), nil
}

// CIDRsOverlap reports whether cidr1 and cidr2 share any addresses.
func CIDRsOverlap(cidr1, cidr2 string) (bool, error) {
	p1, err := netip.ParsePrefix(cidr1)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR %q: %w", cidr1, err)
	}
	p2, err := netip.ParsePrefix(cidr2)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR %q: %w", cidr2, err)
	}
	return p1.Overlaps(p2), nil
}
