// Package netutil provides pure IPv4 address arithmetic; it never touches
// host network state.
package netutil

import (
	"fmt"
	"net/netip"
)

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
