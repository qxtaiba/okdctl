package netutil

import (
	"fmt"
	"net"
	"net/netip"
)

const DefaultVIPLastOctet = 10

func CIDRToNetmask(cidr string) (string, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	if ip4 := network.IP.To4(); ip4 == nil {
		return "", fmt.Errorf("IPv6 CIDR not supported: %q", cidr)
	}
	mask := network.Mask
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3]), nil
}

// ValidateIPRangeInCIDR checks that startIP through startIP+count-1 all
// fall within the given CIDR. Replaces the old /24-only check.
func ValidateIPRangeInCIDR(startIP string, count int, cidr string) error {
	if count <= 0 {
		return fmt.Errorf("count must be positive: %d", count)
	}

	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	start, err := netip.ParseAddr(startIP)
	if err != nil || !start.Is4() {
		return fmt.Errorf("invalid IPv4 address: %s", startIP)
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

func CalculateVMIP(startIP string, index int) (string, error) {
	if index < 0 {
		return "", fmt.Errorf("index cannot be negative: %d", index)
	}

	addr, err := netip.ParseAddr(startIP)
	if err != nil || !addr.Is4() {
		return "", fmt.Errorf("invalid IPv4 address: %s", startIP)
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
		if net.ParseIP(explicitVIP) == nil {
			return "", fmt.Errorf("invalid VIP address: %s", explicitVIP)
		}
		return explicitVIP, nil
	}
	return DeriveVIPFromStaticIP(staticIPStart)
}

func DeriveVIPFromStaticIP(staticIPStart string) (string, error) {
	ip := net.ParseIP(staticIPStart)
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("invalid IPv4 address %q", staticIPStart)
	}
	ip4 := ip.To4()
	return fmt.Sprintf("%d.%d.%d.%d", ip4[0], ip4[1], ip4[2], DefaultVIPLastOctet), nil
}

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
