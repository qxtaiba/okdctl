package netutil

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

const DefaultVIPLastOctet = 10

var ErrInvalidIP = errors.New("invalid IP address")

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

	ipInt := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])

	if uint64(ipInt)+uint64(index) > uint64(^uint32(0)) {
		return "", fmt.Errorf("IP calculation would overflow: %s + %d", startIP, index)
	}

	newIPInt := ipInt + uint32(index)

	newIP := net.IPv4(
		byte(newIPInt>>24),
		byte(newIPInt>>16),
		byte(newIPInt>>8),
		byte(newIPInt),
	)

	return newIP.String(), nil
}

func DeriveVIPFromStaticIP(staticIPStart string) string {
	parts := strings.Split(staticIPStart, ".")
	if len(parts) != 4 {
		return ""
	}
	return fmt.Sprintf("%s.%s.%s.%d", parts[0], parts[1], parts[2], DefaultVIPLastOctet)
}

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

	return s1 <= e2 && s2 <= e1
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

func CIDRsOverlap(cidr1, cidr2 string) bool {
	_, net1, err := net.ParseCIDR(cidr1)
	if err != nil {
		return false
	}
	_, net2, err := net.ParseCIDR(cidr2)
	if err != nil {
		return false
	}

	start1, end1 := cidrRange(net1)
	start2, end2 := cidrRange(net2)

	return ipLessOrEqual(start1, end2) && ipLessOrEqual(start2, end1)
}

func cidrRange(network *net.IPNet) (net.IP, net.IP) {
	first := network.IP.To4()
	if first == nil {
		first = network.IP
	}

	mask := network.Mask
	last := make(net.IP, len(first))
	for i := range first {
		last[i] = first[i] | ^mask[i]
	}

	return first, last
}

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
	return true
}
