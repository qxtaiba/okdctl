// Package netutil provides network utility functions.
package netutil

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// DefaultVIPLastOctet is the default last octet for derived VIP addresses.
const DefaultVIPLastOctet = 10

// ErrInvalidIP is returned when an IP address is invalid.
var ErrInvalidIP = errors.New("invalid IP address")

// CalculateVMIP calculates the IP address for a VM based on a starting IP and index.
func CalculateVMIP(startIP string, index int) (string, error) {
	if index < 0 {
		return "", fmt.Errorf("index cannot be negative: %d", index)
	}

	ip := net.ParseIP(startIP)
	if ip == nil {
		return "", fmt.Errorf("invalid starting IP address: %s", startIP)
	}

	ip = ip.To4()
	if ip == nil {
		return "", fmt.Errorf("only IPv4 addresses are supported: %s", startIP)
	}

	// Convert to uint32 for arithmetic
	ipInt := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])

	// Check for overflow
	if uint64(ipInt)+uint64(index) > uint64(^uint32(0)) {
		return "", fmt.Errorf("IP calculation would overflow: %s + %d", startIP, index)
	}

	newIPInt := ipInt + uint32(index)

	// Convert back to IP
	newIP := net.IPv4(
		byte(newIPInt>>24),
		byte(newIPInt>>16),
		byte(newIPInt>>8),
		byte(newIPInt),
	)

	return newIP.String(), nil
}

// DeriveVIPFromStaticIP derives a VIP address from a static IP.
// Uses DefaultVIPLastOctet (.10) as the last octet (common VIP convention).
// Returns empty string if the input is invalid.
func DeriveVIPFromStaticIP(staticIPStart string) string {
	parts := strings.Split(staticIPStart, ".")
	if len(parts) != 4 {
		return ""
	}
	return fmt.Sprintf("%s.%s.%s.%d", parts[0], parts[1], parts[2], DefaultVIPLastOctet)
}

// IPInCIDR checks if an IP address is within a CIDR range.
// Returns false if either the IP or CIDR is invalid.
func IPInCIDR(ip, cidr string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	return network.Contains(parsedIP)
}

// ipToUint32 converts an IPv4 address to uint32 for comparison.
// Returns 0 and false if the IP is invalid or not IPv4.
func ipToUint32(ip string) (uint32, bool) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0, false
	}
	parsed = parsed.To4()
	if parsed == nil {
		return 0, false
	}
	return uint32(parsed[0])<<24 | uint32(parsed[1])<<16 | uint32(parsed[2])<<8 | uint32(parsed[3]), true
}

// CompareIPs compares two IP addresses.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
// Returns an error if either IP is invalid.
func CompareIPs(a, b string) (int, error) {
	aVal, okA := ipToUint32(a)
	if !okA {
		return 0, fmt.Errorf("%w: %s", ErrInvalidIP, a)
	}
	bVal, okB := ipToUint32(b)
	if !okB {
		return 0, fmt.Errorf("%w: %s", ErrInvalidIP, b)
	}

	if aVal < bVal {
		return -1, nil
	} else if aVal > bVal {
		return 1, nil
	}
	return 0, nil
}

// RangesOverlap checks if two IP ranges overlap.
// Returns false if any of the IPs are invalid.
func RangesOverlap(start1, end1, start2, end2 string) bool {
	s1, ok := ipToUint32(start1)
	if !ok {
		return false
	}
	e1, ok := ipToUint32(end1)
	if !ok {
		return false
	}
	s2, ok := ipToUint32(start2)
	if !ok {
		return false
	}
	e2, ok := ipToUint32(end2)
	if !ok {
		return false
	}

	// Ranges overlap if: start1 <= end2 AND start2 <= end1
	return s1 <= e2 && s2 <= e1
}

// SplitIPv4 splits an IPv4 address into its base (first 3 octets) and last octet.
// Returns an error if the IP is invalid.
func SplitIPv4(ip string) (base string, lastOctet int, err error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", 0, fmt.Errorf("invalid IP address: %s", ip)
	}
	parsed = parsed.To4()
	if parsed == nil {
		return "", 0, fmt.Errorf("only IPv4 addresses are supported: %s", ip)
	}
	base = fmt.Sprintf("%d.%d.%d", parsed[0], parsed[1], parsed[2])
	lastOctet = int(parsed[3])
	return base, lastOctet, nil
}

// ParseIPPool parses a pool string in "start-end" format.
// Returns the start and end IPs, or an error if invalid.
func ParseIPPool(pool string) (start, end string, err error) {
	parts := strings.Split(pool, "-")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid pool format, expected 'start-end': %s", pool)
	}

	start = strings.TrimSpace(parts[0])
	end = strings.TrimSpace(parts[1])

	if net.ParseIP(start) == nil {
		return "", "", fmt.Errorf("invalid start IP in pool: %s", start)
	}
	if net.ParseIP(end) == nil {
		return "", "", fmt.Errorf("invalid end IP in pool: %s", end)
	}

	return start, end, nil
}

// CIDRsOverlap checks if two CIDR ranges overlap.
// Returns false if either CIDR is invalid.
func CIDRsOverlap(cidr1, cidr2 string) bool {
	_, net1, err := net.ParseCIDR(cidr1)
	if err != nil {
		return false
	}
	_, net2, err := net.ParseCIDR(cidr2)
	if err != nil {
		return false
	}

	// Get the first and last IPs of each CIDR
	start1, end1 := cidrRange(net1)
	start2, end2 := cidrRange(net2)

	// Ranges overlap if: start1 <= end2 AND start2 <= end1
	return ipLessOrEqual(start1, end2) && ipLessOrEqual(start2, end1)
}

// cidrRange returns the first and last IP addresses in a CIDR block.
func cidrRange(network *net.IPNet) (net.IP, net.IP) {
	// First IP is the network address
	first := network.IP.To4()
	if first == nil {
		first = network.IP
	}

	// Last IP is obtained by ORing with the inverted mask
	mask := network.Mask
	last := make(net.IP, len(first))
	for i := range first {
		last[i] = first[i] | ^mask[i]
	}

	return first, last
}

// ipLessOrEqual returns true if a <= b (for IPv4).
func ipLessOrEqual(a, b net.IP) bool {
	a = a.To4()
	b = b.To4()
	if a == nil || b == nil {
		return false
	}
	for i := 0; i < 4; i++ {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return true // equal
}
