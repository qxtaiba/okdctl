package steps

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/netutil"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// NetworkingStepDefinition declares the cluster-networking step fields.
var NetworkingStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDNetworking,
	Title:        "network configuration",
	DisplayTitle: "configure cluster networking",
	Description:  "configure cluster networking",
	Sections: []wizard.SectionDefinition{
		{
			Title: "infrastructure network",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "machine_cidr",
					Label:     labelMachineCIDR,
					Default:   "192.168.1.0/24",
					Help:      "network cidr where vms will be deployed, e.g. 192.168.1.0/24",
					Required:  true,
					Validate:  config.ValidateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.MachineCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.MachineCIDR }),
				},
				{
					Key:       fieldGateway,
					Label:     fieldGateway,
					Default:   "192.168.1.1",
					Help:      "network gateway ip address",
					Required:  true,
					Validate:  config.ValidateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.Gateway = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.Gateway }),
				},
				{
					Key:      "dns_servers",
					Label:    labelUpstreamDNS,
					Default:  "192.168.1.1",
					Help:     "comma-separated ip addresses for dnsmasq on bastion — vms resolve through bastion automatically",
					Required: true,
					Validate: validateDNSServers,
					ConfigSet: func(cfg *config.Config, value string) error {
						servers := strings.Split(value, ",")
						cfg.Networking.DNS = make([]string, 0, len(servers))
						for _, dns := range servers {
							dns = strings.TrimSpace(dns)
							if dns != "" {
								cfg.Networking.DNS = append(cfg.Networking.DNS, dns)
							}
						}
						return nil
					},
					ConfigGet: func(cfg *config.Config) string {
						return strings.Join(cfg.Networking.DNS, ", ")
					},
				},
			},
		},
		{
			Title: "kubernetes networks",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "pod_cidr",
					Label:     labelPodCIDR,
					Default:   "10.128.0.0/14",
					Help:      "kubernetes pod network (okd default: 10.128.0.0/14)",
					Required:  true,
					Validate:  config.ValidateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.PodCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.PodCIDR }),
				},
				{
					Key:       "service_cidr",
					Label:     labelServiceCIDR,
					Default:   "172.30.0.0/16",
					Help:      "kubernetes service network (okd default: 172.30.0.0/16)",
					Required:  true,
					Validate:  config.ValidateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.ServiceCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.ServiceCIDR }),
				},
				{
					Key:       "host_prefix",
					Label:     labelHostPrefix,
					Default:   "23",
					Help:      "subnet size per node (smaller = more pods)",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{"20", "21", "22", "23", "24", "25", "26"},
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Networking.HostPrefix = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Networking.HostPrefix }),
				},
			},
		},
		{
			Title: "static ip allocation",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "start_ip",
					Label:     "start ip",
					Default:   DefaultStartIP,
					Help:      "ip address where the bootstrap node boots, e.g. 192.168.1.140 — other nodes follow sequentially and the api vip derives from it",
					Required:  true,
					PairKey:   "static_ip",
					Validate:  config.ValidateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.StaticIP.Start = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.StaticIP.Start }),
				},
				{
					Key:       fieldInterface,
					Label:     fieldInterface,
					Default:   "ens18",
					Help:      "network interface inside vms — ens18 is the proxmox/virtio default; use ip link in a vm to verify",
					Required:  true,
					PairKey:   "static_ip",
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.StaticIP.Interface = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.StaticIP.Interface }),
				},
			},
		},
		{
			Title: "load balancing",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "bastion_ip",
					Label:     "bastion ip",
					Default:   "192.168.1.20",
					Help:      "ip of this machine (runs haproxy + dnsmasq — vms use this for dns resolution)",
					Required:  true,
					PairKey:   "load_balancing",
					Validate:  config.ValidateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.Bastion.IP = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.Bastion.IP }),
				},
				{
					Key:         "vip",
					Label:       labelAPIVIP,
					Default:     "",
					Placeholder: "auto",
					PairKey:     "load_balancing",
					Help:        "virtual ip for kubernetes api — leave blank to auto-derive from static ip start",
					Validate: func(value string) error {
						if value == "" {
							return nil
						}
						return config.ValidateIP(value)
					},
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.Bastion.VIP = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.Bastion.VIP }),
				},
			},
		},
	},
	Validate: func(values map[string]string) error {
		machineCIDR := values["machine_cidr"]
		podCIDR := values["pod_cidr"]
		serviceCIDR := values["service_cidr"]

		if overlap, ok := cidrsOverlapIfValid(machineCIDR, podCIDR); ok && overlap {
			return errors.New("machine cidr and pod cidr must not overlap — widen or move one of the ranges")
		}
		if overlap, ok := cidrsOverlapIfValid(machineCIDR, serviceCIDR); ok && overlap {
			return errors.New("machine cidr and service cidr must not overlap — widen or move one of the ranges")
		}
		if overlap, ok := cidrsOverlapIfValid(podCIDR, serviceCIDR); ok && overlap {
			return errors.New("pod cidr and service cidr must not overlap — widen or move one of the ranges")
		}
		if err := config.ValidateGatewayInCIDR(values[fieldGateway], machineCIDR); err != nil {
			return err
		}
		return nil
	},
	Apply: func(_ *wizard.DataDrivenStep, cfg *config.Config) error {
		cfg.Networking.StaticIP.DNS = cfg.Networking.Bastion.IP
		netmask, err := netutil.CIDRToNetmask(cfg.Networking.MachineCIDR)
		if err != nil {
			return err
		}
		cfg.Networking.StaticIP.Netmask = netmask
		return nil
	},
}

// cidrsOverlapIfValid reports whether a and b overlap, with checked false
// when either fails to parse — the per-field CIDR validator already shows
// its own clean format error for a malformed value, so the overlap check
// stays silent rather than surfacing netutil's wrapped netip parse error.
func cidrsOverlapIfValid(a, b string) (overlap, checked bool) {
	if !config.IsValidCIDR(a) || !config.IsValidCIDR(b) {
		return false, false
	}
	overlap, err := netutil.CIDRsOverlap(a, b)
	return overlap, err == nil
}

// NewNetworkingStep returns the networking wizard step, with an allocation
// preview once capacity is non-nil.
func NewNetworkingStep(capacity *WizardCapacitySnapshot) *wizard.DataDrivenStep {
	step := wizard.NewDataDrivenStep(&NetworkingStepDefinition)
	if capacity == nil {
		return step
	}
	step.WithExtraContentFunc("allocation preview", func(s *wizard.DataDrivenStep, _ int) string {
		cpCount, workerCount, known := capacity.counts()
		if !known || capacity.discovery == nil {
			return ""
		}
		values := make(map[string]string, 8)
		for _, key := range []string{"machine_cidr", fieldGateway, "start_ip", "bastion_ip", "vip"} {
			values[key] = s.Value(key)
		}
		if capacity.cfg != nil {
			values["ignition_ip"] = capacity.cfg.HTTPServer.IgnitionServerIP
			if capacity.cfg.Provider.Proxmox != nil {
				values["proxmox_host"] = capacity.cfg.Provider.Proxmox.Host
			}
		}
		return renderNetworkAllocationPreview(values, cpCount, workerCount)
	})
	return step
}

func renderNetworkAllocationPreview(values map[string]string, controlPlaneCount, workerCount int) string {
	warning := lipgloss.NewStyle().Foreground(tui.ColorWarning())
	startText := values["start_ip"]
	start, err := netip.ParseAddr(startText)
	if err != nil || !start.Is4() {
		return warning.Render("invalid start IP — allocation unavailable")
	}
	count := 1 + controlPlaneCount + workerCount
	var rows []string
	ips := make([]string, count)
	for i := range count {
		ip, calcErr := netutil.CalculateVMIP(startText, i)
		if calcErr != nil {
			return warning.Render("static IP allocation invalid — preview unavailable")
		}
		ips[i] = ip
	}
	rows = append(rows, "bootstrap "+ips[0])
	if controlPlaneCount > 0 {
		rows = append(rows, allocationGroup("masters", ips[1:1+controlPlaneCount]))
	}
	for i := 0; i < workerCount; i++ {
		rows = append(rows, fmt.Sprintf("worker%d %s", i, ips[1+controlPlaneCount+i]))
	}
	cidr := values["machine_cidr"]
	if config.ValidateCIDR(cidr) != nil {
		rows = append(rows, warning.Render("invalid machine CIDR"))
	} else {
		if err := netutil.ValidateIPRangeInCIDR(startText, count, cidr); err != nil {
			rows = append(rows, warning.Render("static IP range outside machine CIDR"))
		}
		if values[fieldGateway] != "" && config.ValidateGatewayInCIDR(values[fieldGateway], cidr) != nil {
			rows = append(rows, warning.Render("gateway invalid for machine CIDR: "+values[fieldGateway]))
		}
	}
	if explicit := values["vip"]; explicit != "" {
		if _, err := netutil.ResolveVIP(explicit, startText); err != nil {
			rows = append(rows, warning.Render("api vip invalid: "+explicit))
		} else {
			rows = append(rows, "api vip "+explicit)
			if config.ValidateCIDR(cidr) == nil && !addressInCIDR(explicit, cidr) {
				rows = append(rows, warning.Render("api vip outside machine CIDR: "+explicit))
			}
		}
	} else if vip, err := netutil.ResolveVIP("", startText); err == nil {
		rows = append(rows, "api vip "+vip+" (auto)")
		if config.ValidateCIDR(cidr) == nil && !addressInCIDR(vip, cidr) {
			rows = append(rows, warning.Render("api vip outside machine CIDR: "+vip))
		}
	}
	if bastion := values["bastion_ip"]; bastion != "" {
		if _, err := netip.ParseAddr(bastion); err != nil {
			rows = append(rows, warning.Render("vm dns target invalid: "+bastion))
		} else {
			rows = append(rows, "vm dns → bastion "+bastion)
			if config.ValidateCIDR(cidr) == nil && !addressInCIDR(bastion, cidr) {
				rows = append(rows, warning.Render("bastion outside machine CIDR: "+bastion))
			}
		}
	}
	if gateway := values[fieldGateway]; gateway != "" {
		if _, err := netip.ParseAddr(gateway); err != nil {
			rows = append(rows, warning.Render("gateway invalid: "+gateway))
		}
	}
	if collision := allocationCollision(ips, values[fieldGateway], values["bastion_ip"], allocationVIP(values, startText), values["ignition_ip"], proxmoxHostIP(values["proxmox_host"])); collision != "" {
		rows = append(rows, warning.Render("collision at "+collision))
	}
	return strings.Join(rows, "\n")
}

func addressInCIDR(address, cidr string) bool {
	addr, addrErr := netip.ParseAddr(address)
	prefix, prefixErr := netip.ParsePrefix(cidr)
	return addrErr == nil && prefixErr == nil && prefix.Contains(addr)
}

func proxmoxHostIP(host string) string {
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	if parsed, err := netip.ParseAddr(host); err == nil {
		return parsed.String()
	}
	if name, _, err := net.SplitHostPort(host); err == nil {
		if parsed, parseErr := netip.ParseAddr(name); parseErr == nil {
			return parsed.String()
		}
	}
	return ""
}

func allocationGroup(label string, ips []string) string {
	if len(ips) == 1 {
		return fmt.Sprintf("%s %s", strings.TrimSuffix(label, "s"), ips[0])
	}
	return fmt.Sprintf("%s %s–%s", label, ips[0], ips[len(ips)-1])
}

func allocationVIP(values map[string]string, start string) string {
	vip, err := netutil.ResolveVIP(values["vip"], start)
	if err != nil {
		return ""
	}
	return vip
}

func allocationCollision(ips []string, reserved ...string) string {
	used := make(map[netip.Addr]string, len(ips)+len(reserved))
	for i, value := range ips {
		addr, err := netip.ParseAddr(value)
		if err != nil {
			continue
		}
		used[addr] = fmt.Sprintf("vm %d", i)
	}
	for _, value := range reserved {
		addr, err := netip.ParseAddr(value)
		if err != nil || value == "" {
			continue
		}
		if _, exists := used[addr]; exists {
			return addr.String()
		}
		used[addr] = "reserved"
	}
	return ""
}
