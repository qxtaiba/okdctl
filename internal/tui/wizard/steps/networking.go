package steps

import (
	"fmt"
	"net/netip"
	"slices"
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
	Sections:     networkingSections,
	Validate:     networkingCrossFields,
}

var networkingSections = []wizard.SectionDefinition{
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
}

var networkingAddressKeys = []string{
	config.FieldNetworkingGateway, config.FieldNetworkingAPIVIP,
	config.FieldNetworkingIngressVIP, config.FieldNetworkingNodeIPStart,
}

// networkingCrossFields validates against the compiled-in defaults;
// NewNetworkingStep swaps in the session config.
func networkingCrossFields(values map[string]string) error {
	return networkingValidate(values, config.DefaultConfig())
}

// networkingValidate applies values onto a copy of cfg and reports the first
// config.Validate finding on one of this step's fields, so the wizard and the
// loader share one rule set.
func networkingValidate(values map[string]string, cfg *config.Config) error {
	scratch := *cfg
	scratch.Networking.DNS = slices.Clone(scratch.Networking.DNS)
	scratch.Networking.NTP = slices.Clone(scratch.Networking.NTP)
	// A value its own field validator rejects is that field's to report.
	wellFormed := make(map[string]bool)
	for i := range networkingSections {
		for j := range networkingSections[i].Fields {
			field := &networkingSections[i].Fields[j]
			v, ok := values[field.Key]
			if !ok || field.ConfigSet == nil {
				continue
			}
			_ = field.ConfigSet(&scratch, v)
			wellFormed[field.Key] = field.Validate == nil || field.Validate(v) == nil
		}
	}
	for _, e := range scratch.Validate().Errors {
		key, _, _ := strings.Cut(e.Field, "[")
		if wellFormed[key] {
			return wizard.NewCrossFieldError(fmt.Errorf("%s: %s", fieldLabel(key), e.Message), key)
		}
	}
	return nil
}

// NewNetworkingStep returns the networking step with an allocation preview
// once capacity carries the session config.
func NewNetworkingStep(capacity *WizardCapacitySnapshot) *wizard.DataDrivenStep {
	if capacity == nil {
		return wizard.NewDataDrivenStep(&NetworkingStepDefinition)
	}
	def := NetworkingStepDefinition
	def.Validate = func(values map[string]string) error {
		if capacity.cfg == nil {
			return networkingCrossFields(values)
		}
		return networkingValidate(values, capacity.cfg)
	}
	step := wizard.NewDataDrivenStep(&def)
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
// indices; a pool with holes lists each address.
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
