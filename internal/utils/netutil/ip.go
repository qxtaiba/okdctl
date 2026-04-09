package netutil

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
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

// ValidateIPRangeWithin24 ensures that startIP + count - 1 does not
// overflow the /24 containing startIP, i.e. that the last octet of the final
// address stays <= 255. This is the shared precondition for any code that
// derives a sequence of node IPs from a start address (setup/nodes.go,
// dns/dns.go, wizard validators). Returns nil if the range fits.
func ValidateIPRangeWithin24(startIP string, count int) error {
	if count <= 0 {
		return fmt.Errorf("count must be positive: %d", count)
	}
	_, lastOctet, err := SplitIPv4(startIP)
	if err != nil {
		return err
	}
	highest := lastOctet + count - 1
	if highest > 255 {
		return fmt.Errorf("IP range insufficient: start %q + %d addresses overflows subnet (last octet would be %d)", startIP, count, highest)
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

	newInt := ipInt + uint32(index)
	result := netip.AddrFrom4([4]byte{
		byte(newInt >> 24), byte(newInt >> 16), byte(newInt >> 8), byte(newInt),
	})
	return result.String(), nil
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
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false, fmt.Errorf("invalid IP address %q", ip)
	}

	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	return network.Contains(parsedIP), nil
}

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
