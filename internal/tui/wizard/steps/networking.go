package steps

import (
	"errors"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
)

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
					Label:     "machine cidr",
					Default:   "192.168.1.0/24",
					Help:      "network where vms will be deployed",
					Required:  true,
					Validate:  config.ValidateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.MachineCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.MachineCIDR }),
				},
				{
					Key:       "gateway",
					Label:     "gateway",
					Default:   "192.168.1.1",
					Help:      "network gateway ip address",
					Required:  true,
					Validate:  config.ValidateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.Gateway = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.Gateway }),
				},
				{
					Key:      "dns_servers",
					Label:    "upstream dns",
					Default:  "192.168.1.1",
					Help:     "upstream dns for dnsmasq on bastion — vms resolve through bastion automatically",
					Required: true,
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
					Label:     "pod cidr",
					Default:   "10.128.0.0/14",
					Help:      "kubernetes pod network (okd default: 10.128.0.0/14)",
					Required:  true,
					Validate:  config.ValidateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.PodCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.PodCIDR }),
				},
				{
					Key:       "service_cidr",
					Label:     "service cidr",
					Default:   "172.30.0.0/16",
					Help:      "kubernetes service network (okd default: 172.30.0.0/16)",
					Required:  true,
					Validate:  config.ValidateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.ServiceCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.ServiceCIDR }),
				},
				{
					Key:       "host_prefix",
					Label:     "host prefix",
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
					Default:   "192.168.1.100",
					Help:      "first ip for node allocation",
					Required:  true,
					Validate:  config.ValidateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.StaticIP.Start = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.StaticIP.Start }),
				},
				{
					Key:       "interface",
					Label:     "interface",
					Default:   "ens18",
					Help:      "network interface inside vms — ens18 is the proxmox/virtio default; use ip link in a vm to verify",
					Required:  true,
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
					Validate:  config.ValidateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.Bastion.IP = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.Bastion.IP }),
				},
				{
					Key:     "vip",
					Label:   "api vip",
					Default: "",
					Help:    "virtual ip for kubernetes api — leave blank to auto-derive from static ip start",
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

		if overlap, err := netutil.CIDRsOverlap(machineCIDR, podCIDR); err != nil {
			return err
		} else if overlap {
			return errors.New("machine cidr and pod cidr must not overlap")
		}
		if overlap, err := netutil.CIDRsOverlap(machineCIDR, serviceCIDR); err != nil {
			return err
		} else if overlap {
			return errors.New("machine cidr and service cidr must not overlap")
		}
		if overlap, err := netutil.CIDRsOverlap(podCIDR, serviceCIDR); err != nil {
			return err
		} else if overlap {
			return errors.New("pod cidr and service cidr must not overlap")
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

func NewNetworkingStep() (*wizard.DataDrivenStep, *wizard.DataDrivenStep) {
	step := wizard.NewDataDrivenStep(&NetworkingStepDefinition)
	return step, step
}
