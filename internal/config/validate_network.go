package config

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

const (
	minHostPrefix = 20
	maxHostPrefix = 28
)

func (c *Config) validateNetwork(r *ValidationResult, nodes []nodeRef) {
	n := &c.Networking
	machine := parsePrefix(r, FieldNetworkingMachineCIDR, n.MachineCIDR)
	pod := parsePrefix(r, FieldNetworkingPodCIDR, n.PodCIDR)
	service := parsePrefix(r, FieldNetworkingServiceCIDR, n.ServiceCIDR)
	// nmstate routes the default gateway on-link; one outside the prefix never comes up.
	gateway := parseAddr(r, FieldNetworkingGateway, n.Gateway)
	if gateway.IsValid() && machine.IsValid() && !machine.Contains(gateway) {
		r.addf(FieldNetworkingGateway, "%s is outside machine_cidr %s", gateway, machine)
	}
	if len(n.DNS) == 0 {
		r.AddError(FieldNetworkingDNS, "list at least one resolver")
	}
	for i, d := range n.DNS {
		parseAddr(r, fmt.Sprintf("%s[%d]", FieldNetworkingDNS, i), d)
	}
	c.validateVIPs(r, machine, nodes)
	c.validateDerivedAddresses(r, machine, nodes)
	if pod.IsValid() && machine.IsValid() && pod.Overlaps(machine) {
		r.addf(FieldNetworkingPodCIDR, "%s overlaps machine_cidr %s", pod, machine)
	}
	if service.IsValid() && machine.IsValid() && service.Overlaps(machine) {
		r.addf(FieldNetworkingServiceCIDR, "%s overlaps machine_cidr %s", service, machine)
	}
	if service.IsValid() && pod.IsValid() && service.Overlaps(pod) {
		r.addf(FieldNetworkingServiceCIDR, "%s overlaps pod_cidr %s", service, pod)
	}
	switch {
	case n.HostPrefix < minHostPrefix || n.HostPrefix > maxHostPrefix:
		r.addf(FieldNetworkingHostPrefix, "%d must be between %d and %d",
			n.HostPrefix, minHostPrefix, maxHostPrefix)
	case pod.IsValid() && n.HostPrefix < pod.Bits():
		r.addf(FieldNetworkingHostPrefix, "%d must be at least the pod_cidr prefix length %d",
			n.HostPrefix, pod.Bits())
	}
}

func (c *Config) validateVIPs(r *ValidationResult, machine netip.Prefix, nodes []nodeRef) {
	n := &c.Networking
	api := parseAddr(r, FieldNetworkingAPIVIP, n.APIVIP)
	ingress := parseAddr(r, FieldNetworkingIngressVIP, n.IngressVIP)
	gateway, _ := netip.ParseAddr(n.Gateway)
	for _, vip := range []struct {
		field string
		addr  netip.Addr
	}{{FieldNetworkingAPIVIP, api}, {FieldNetworkingIngressVIP, ingress}} {
		if !vip.addr.IsValid() {
			continue
		}
		if machine.IsValid() && !machine.Contains(vip.addr) {
			r.addf(vip.field, "%s is outside machine_cidr %s", vip.addr, machine)
		}
		if edge := edgeOf(machine, vip.addr); edge != "" {
			r.addf(vip.field, "%s is the %s address of machine_cidr %s", vip.addr, edge, machine)
		}
		if vip.addr == gateway {
			r.addf(vip.field, "%s equals gateway", vip.addr)
		}
		for i, d := range n.DNS {
			if resolver, err := netip.ParseAddr(d); err == nil && vip.addr == resolver {
				r.addf(vip.field, "%s equals dns[%d]", vip.addr, i)
			}
		}
		for i := range nodes {
			if node := &nodes[i]; node.derived && node.ip == vip.addr {
				r.addf(vip.field, "%s equals the address of %s", vip.addr, node.name)
			}
		}
	}
	if api.IsValid() && api == ingress {
		r.addf(FieldNetworkingIngressVIP, "%s equals api_vip", ingress)
	}
}

// validateDerivedAddresses checks node_ip_start and every derived address:
// inside machine_cidr (the first one outside is reported, the rest follow
// it) and clear of the gateway, the resolvers and the prefix edges;
// overrides have their own rules.
func (c *Config) validateDerivedAddresses(
	r *ValidationResult, machine netip.Prefix, nodes []nodeRef,
) {
	n := &c.Networking
	start := parseAddr(r, FieldNetworkingNodeIPStart, n.NodeIPStart)
	if start.IsValid() && machine.IsValid() && !machine.Contains(start) {
		r.addf(FieldNetworkingNodeIPStart, "%s is outside machine_cidr %s", start, machine)
		return
	}
	gateway, _ := netip.ParseAddr(n.Gateway)
	outside := false
	for i := range nodes {
		node := &nodes[i]
		if !node.derived {
			continue
		}
		if machine.IsValid() && !machine.Contains(node.ip) {
			if !outside {
				r.addf(FieldNetworkingNodeIPStart, "%s derives %s, outside machine_cidr %s",
					node.name, node.ip, machine)
			}
			outside = true
			continue
		}
		if node.ip == gateway {
			r.addf(FieldNetworkingNodeIPStart, "%s and gateway share %s", node.name, node.ip)
		}
		for k, d := range n.DNS {
			if resolver, err := netip.ParseAddr(d); err == nil && node.ip == resolver {
				r.addf(FieldNetworkingNodeIPStart, "%s and dns[%d] share %s", node.name, k, node.ip)
			}
		}
		if edge := edgeOf(machine, node.ip); edge != "" {
			r.addf(FieldNetworkingNodeIPStart, "%s lands on %s, the %s address of machine_cidr %s",
				node.name, node.ip, edge, machine)
		}
	}
}

// edgeOf names the edge of p that addr sits on, "network" or "broadcast",
// or "" for an ordinary host address.
func edgeOf(p netip.Prefix, addr netip.Addr) string {
	switch network, broadcast := edges(p); addr {
	case network:
		return "network"
	case broadcast:
		return "broadcast"
	}
	return ""
}

// edges returns the network and broadcast addresses of an IPv4 prefix
// shorter than /31; a /31 or /32 reserves neither.
func edges(p netip.Prefix) (network, broadcast netip.Addr) {
	if !p.IsValid() || !p.Addr().Is4() || p.Bits() >= 31 {
		return netip.Addr{}, netip.Addr{}
	}
	first := p.Masked().Addr().As4()
	last := first
	hostBits := uint32(1)<<(32-p.Bits()) - 1
	binary.BigEndian.PutUint32(last[:], binary.BigEndian.Uint32(first[:])|hostBits)
	return netip.AddrFrom4(first), netip.AddrFrom4(last)
}

func parsePrefix(r *ValidationResult, field, value string) netip.Prefix {
	p, err := netip.ParsePrefix(value)
	if err != nil {
		r.addf(field, "%q is not a CIDR such as 192.168.1.0/24", value)
		return netip.Prefix{}
	}
	return p.Masked()
}

func parseAddr(r *ValidationResult, field, value string) netip.Addr {
	a, err := netip.ParseAddr(value)
	if err != nil {
		r.addf(field, "%q is not an IP address", value)
		return netip.Addr{}
	}
	return a
}
