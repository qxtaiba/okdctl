package config

import (
	"fmt"
	"net/netip"

	"github.com/qxtaiba/okdctl/internal/netutil"
)

const (
	minHostPrefix = 20
	maxHostPrefix = 28
)

func (c *Config) validateNetwork(r *ValidationResult) {
	n := &c.Networking
	machine := parsePrefix(r, FieldNetworkingMachineCIDR, n.MachineCIDR)
	pod := parsePrefix(r, FieldNetworkingPodCIDR, n.PodCIDR)
	service := parsePrefix(r, FieldNetworkingServiceCIDR, n.ServiceCIDR)
	parseAddr(r, FieldNetworkingGateway, n.Gateway)
	if len(n.DNS) == 0 {
		r.AddError(FieldNetworkingDNS, "list at least one resolver")
	}
	for i, d := range n.DNS {
		parseAddr(r, fmt.Sprintf("%s[%d]", FieldNetworkingDNS, i), d)
	}
	c.validateVIPs(r, machine)
	c.validateNodeRange(r, machine)
	if pod.IsValid() && machine.IsValid() && pod.Overlaps(machine) {
		r.addf(FieldNetworkingPodCIDR, "%s overlaps machine_cidr %s", pod, machine)
	}
	if service.IsValid() && machine.IsValid() && service.Overlaps(machine) {
		r.addf(FieldNetworkingServiceCIDR, "%s overlaps machine_cidr %s", service, machine)
	}
	if service.IsValid() && pod.IsValid() && service.Overlaps(pod) {
		r.addf(FieldNetworkingServiceCIDR, "%s overlaps pod_cidr %s", service, pod)
	}
	if n.HostPrefix < minHostPrefix || n.HostPrefix > maxHostPrefix {
		r.addf(FieldNetworkingHostPrefix, "%d must be between %d and %d", n.HostPrefix, minHostPrefix, maxHostPrefix)
	}
}

func (c *Config) validateVIPs(r *ValidationResult, machine netip.Prefix) {
	n := &c.Networking
	api := parseAddr(r, FieldNetworkingAPIVIP, n.APIVIP)
	ingress := parseAddr(r, FieldNetworkingIngressVIP, n.IngressVIP)
	gateway, _ := netip.ParseAddr(n.Gateway)
	nodes := c.nodes()
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

// validateNodeRange checks node_ip_start and the last derived node address;
// a CIDR is contiguous, so both inside means every node address is inside.
func (c *Config) validateNodeRange(r *ValidationResult, machine netip.Prefix) {
	start := parseAddr(r, FieldNetworkingNodeIPStart, c.Networking.NodeIPStart)
	if !start.IsValid() || !machine.IsValid() {
		return
	}
	if !machine.Contains(start) {
		r.addf(FieldNetworkingNodeIPStart, "%s is outside machine_cidr %s", start, machine)
		return
	}
	last, err := netutil.CalculateVMIP(c.Networking.NodeIPStart, c.highestIPOffset())
	if err != nil {
		r.addf(FieldNetworkingNodeIPStart, "derive the last node address: %v", err)
		return
	}
	if end := netip.MustParseAddr(last); !machine.Contains(end) {
		r.addf(FieldNetworkingNodeIPStart, "node range %s-%s leaves machine_cidr %s", start, end, machine)
	}
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
