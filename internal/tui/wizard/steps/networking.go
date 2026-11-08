package steps

import (
	"fmt"
	"net/netip"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

const pairKeyVIPs = "vips"

// NetworkingStepDefinition declares the machine network, VIP and node
// address fields.
var NetworkingStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDNetworking,
	Title:        "network configuration",
	DisplayTitle: "configure cluster networking",
	Description:  "lay out the machine network, the vips and the node addresses",
	Sections: []wizard.SectionDefinition{
		{
			Title: "machine network",
			Fields: []wizard.FieldDefinition{
				{
					Key:       config.FieldNetworkingMachineCIDR,
					Label:     "machine cidr",
					Default:   "192.168.1.0/24",
					Help:      "network the vms live on, e.g. 192.168.1.0/24",
					Required:  true,
					Validate:  validateCIDR,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.MachineCIDR = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.MachineCIDR }),
				},
				{
					Key:       config.FieldNetworkingGateway,
					Label:     "gateway",
					Default:   "192.168.1.1",
					Help:      "default gateway ip address inside the machine cidr",
					Required:  true,
					Validate:  validateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.Gateway = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.Gateway }),
				},
				{
					Key:       config.FieldNetworkingDNS,
					Label:     "dns servers",
					Default:   "192.168.1.1",
					Help:      "comma-separated resolver ip addresses the nodes use, e.g. 192.168.1.1, 1.1.1.1",
					Required:  true,
					Validate:  validateIPList,
					ConfigSet: setList(func(c *config.Config, v []string) { c.Networking.DNS = v }),
					ConfigGet: getList(func(c *config.Config) []string { return c.Networking.DNS }),
				},
			},
		},
		{
			Title: "virtual ips",
			Fields: []wizard.FieldDefinition{
				{
					Key:       config.FieldNetworkingAPIVIP,
					Label:     "api vip",
					Default:   "192.168.1.158",
					Help:      "unused ip address in the machine cidr for the kubernetes api, e.g. 192.168.1.158",
					Required:  true,
					PairKey:   pairKeyVIPs,
					Validate:  validateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.APIVIP = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.APIVIP }),
				},
				{
					Key:       config.FieldNetworkingIngressVIP,
					Label:     "ingress vip",
					Default:   "192.168.1.159",
					Help:      "unused ip address in the machine cidr for the ingress router, e.g. 192.168.1.159",
					Required:  true,
					PairKey:   pairKeyVIPs,
					Validate:  validateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.IngressVIP = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.IngressVIP }),
				},
			},
		},
		{
			Title: "node addresses",
			Fields: []wizard.FieldDefinition{
				{
					Key:       config.FieldNetworkingNodeIPStart,
					Label:     "node ip start",
					Default:   "192.168.1.150",
					Help:      "first node address — masters take +0..2 and workers +10 onward, e.g. 192.168.1.150",
					Required:  true,
					Validate:  validateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.NodeIPStart = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.NodeIPStart }),
				},
				{
					Key:         fieldNTP,
					Label:       "ntp servers",
					Placeholder: "pool.ntp.org",
					Help:        "comma-separated hostnames or ips for chrony — blank keeps the default pool",
					ConfigSet:   setList(func(c *config.Config, v []string) { c.Networking.NTP = v }),
					ConfigGet:   getList(func(c *config.Config) []string { return c.Networking.NTP }),
				},
			},
		},
	},
	Validate: networkingCrossFields,
}

var networkingAddressKeys = []string{
	config.FieldNetworkingGateway, config.FieldNetworkingAPIVIP,
	config.FieldNetworkingIngressVIP, config.FieldNetworkingNodeIPStart,
}

// networkingCrossFields checks what no single field can: every address
// inside the machine cidr and no two of them equal. Unparseable values are
// left to their own field validator.
func networkingCrossFields(values map[string]string) error {
	cidr, err := netip.ParsePrefix(values[config.FieldNetworkingMachineCIDR])
	if err != nil {
		return nil //nolint:nilerr // the machine cidr's own field validator reports the malformed value
	}
	seen := make(map[string]string, len(networkingAddressKeys))
	for _, key := range networkingAddressKeys {
		addr, err := netip.ParseAddr(values[key])
		if err != nil {
			continue
		}
		if !cidr.Contains(addr) {
			return wizard.NewCrossFieldError(
				fmt.Errorf("%s %s is outside machine cidr %s — pick an address inside it or widen the cidr", fieldLabel(key), addr, cidr.Masked()),
				key, config.FieldNetworkingMachineCIDR)
		}
		if other, dup := seen[addr.String()]; dup {
			return wizard.NewCrossFieldError(
				fmt.Errorf("%s and %s both use %s — every address must be distinct", fieldLabel(other), fieldLabel(key), addr),
				other, key)
		}
		seen[addr.String()] = key
	}
	return nil
}

// NewNetworkingStep returns the networking wizard step, with an allocation
// preview once capacity carries the session config.
func NewNetworkingStep(capacity *WizardCapacitySnapshot) *wizard.DataDrivenStep {
	step := wizard.NewDataDrivenStep(&NetworkingStepDefinition)
	if capacity == nil {
		return step
	}
	step.WithExtraContentFunc("allocation preview", func(s *wizard.DataDrivenStep, _ int) string {
		if capacity.cfg == nil {
			return ""
		}
		values := make(map[string]string, 5)
		for _, key := range append([]string{config.FieldNetworkingMachineCIDR}, networkingAddressKeys...) {
			values[key] = s.Value(key)
		}
		return renderNetworkAllocationPreview(values, capacity.cfg.WorkerIndices())
	})
	return step
}

// renderNetworkAllocationPreview lays out the master block and the worker
// indices the config produces; a pool with nodes: holes lists each address.
func renderNetworkAllocationPreview(values map[string]string, workers []int) string {
	warning := lipgloss.NewStyle().Foreground(tui.ColorWarning())
	start, err := netip.ParseAddr(values[config.FieldNetworkingNodeIPStart])
	if err != nil || !start.Is4() {
		return warning.Render("invalid node ip start — allocation unavailable")
	}
	last := config.MasterCount - 1
	rows := []string{"masters " + addressRange(start, 0, last)}
	if len(workers) > 0 {
		first, lastWorker := workers[0], workers[len(workers)-1]
		last = config.NodeIPOffset(config.RoleWorker, lastWorker)
		addresses := addressRange(start, config.NodeIPOffset(config.RoleWorker, first), last)
		if lastWorker-first != len(workers)-1 {
			each := make([]string, len(workers))
			for i, w := range workers {
				each[i] = addressAt(start, config.NodeIPOffset(config.RoleWorker, w)).String()
			}
			addresses = strings.Join(each, ", ")
		}
		rows = append(rows, "workers "+addresses)
	}
	for _, key := range []string{config.FieldNetworkingAPIVIP, config.FieldNetworkingIngressVIP} {
		if v := values[key]; v != "" {
			rows = append(rows, fieldLabel(key)+" "+v)
		}
	}
	if cidr, err := netip.ParsePrefix(values[config.FieldNetworkingMachineCIDR]); err == nil {
		if !cidr.Contains(start) || !cidr.Contains(addressAt(start, last)) {
			rows = append(rows, warning.Render("node range leaves machine cidr "+cidr.Masked().String()))
		}
	}
	return strings.Join(rows, "\n")
}

func addressRange(start netip.Addr, from, to int) string {
	if from == to {
		return addressAt(start, from).String()
	}
	return addressAt(start, from).String() + "–" + addressAt(start, to).String()
}

func addressAt(start netip.Addr, offset int) netip.Addr {
	for range offset {
		start = start.Next()
	}
	return start
}
