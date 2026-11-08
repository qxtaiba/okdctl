package steps

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

const cpuTypeHost = "host"

func proxmoxGet(get func(p *config.ProxmoxConfig) string) wizard.ConfigGetter {
	return func(cfg *config.Config) string { return get(&cfg.Provider.Proxmox) }
}

func proxmoxSet(set func(p *config.ProxmoxConfig, v string)) wizard.ConfigSetter {
	return wizard.SetString(func(cfg *config.Config, v string) { set(&cfg.Provider.Proxmox, v) })
}

// ProxmoxStepDefinition declares the Proxmox connection and placement fields.
var ProxmoxStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDProxmox,
	Title:        "proxmox configuration",
	DisplayTitle: "configure proxmox ve",
	Description:  "connect to proxmox and pick where the vms live",
	Sections: []wizard.SectionDefinition{
		{
			Title: "connection",
			Fields: []wizard.FieldDefinition{
				{
					Key:         config.FieldProxmoxEndpoint,
					Label:       "endpoint",
					Default:     "https://192.168.1.100:8006",
					Placeholder: "https://192.168.1.100:8006",
					Help:        "proxmox api url with scheme and host, e.g. https://192.168.1.100:8006",
					Required:    true,
					Validate:    validateEndpoint,
					ConfigSet:   proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Endpoint = v }),
					ConfigGet:   proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Endpoint }),
				},
				{
					Key:   fieldAPIToken,
					Label: "api token",
					Help:  "user@realm!tokenname=secret, as PROXMOX_VE_API_TOKEN expects — saved to okdctl.env, never okdctl.yaml; blank keeps the saved token",
					Type:  wizard.FieldTypePassword,
				},
				{
					Key:     fieldInsecure,
					Label:   "skip tls verify",
					Default: valNo,
					Help:    "skip tls certificate verification — yes only for self-signed certs; saved to okdctl.env as PROXMOX_VE_INSECURE",
					Type:    wizard.FieldTypeSelect,
					Options: []string{valNo, valYes},
					// seeded from PROXMOX_VE_INSECURE, which init loads before the wizard
					ConfigGet: getBool(func(*config.Config) bool { return credentials.GetProxmoxCredentials(nil).Insecure }),
				},
			},
		},
		{
			Title: "placement",
			Fields: []wizard.FieldDefinition{
				{
					Key:       config.FieldProxmoxStorage,
					Label:     "storage",
					Default:   "local-lvm",
					Help:      "storage pool for vm disks (content: images)",
					Required:  true,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Storage = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Storage }),
				},
				{
					Key:       config.FieldProxmoxISOStorage,
					Label:     "iso storage",
					Default:   "local",
					Help:      "storage the agent iso is uploaded to (content: iso)",
					Required:  true,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.ISOStorage = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.ISOStorage }),
				},
				{
					Key:       config.FieldProxmoxBridge,
					Label:     "bridge",
					Default:   "vmbr0",
					Help:      "network bridge for the vm nics, e.g. vmbr0",
					Required:  true,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Bridge = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Bridge }),
				},
			},
		},
		{
			Title:       "advanced",
			Collapsible: true,
			FoldSummary: func(values map[string]string) []tui.FactRow {
				vlan := values[config.FieldProxmoxVLAN]
				if vlan == "" || vlan == "0" {
					vlan = "untagged"
				}
				return []tui.FactRow{
					{Key: labelVLAN, Value: vlan},
					{Key: labelCPUType, Value: values[config.FieldProxmoxCPUType]},
					{Key: "ha", Value: values[fieldHA]},
					{Key: labelVMIDBase, Value: values[config.FieldProxmoxVMIDBase]},
				}
			},
			Fields: []wizard.FieldDefinition{
				{
					Key:       config.FieldProxmoxVLAN,
					Label:     labelVLAN,
					Default:   "0",
					Help:      "vlan tag for the vm nics — 0 is untagged, else 1-4094",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  intRange(0, 4094),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Provider.Proxmox.VLAN = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Provider.Proxmox.VLAN }),
				},
				{
					Key:       config.FieldProxmoxCPUType,
					Label:     labelCPUType,
					Default:   cpuTypeHost,
					Help:      "host gives best performance; x86-64-v3 or newer allows live migration",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{cpuTypeHost, "x86-64-v3", "x86-64-v4"},
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.CPUType = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.CPUType }),
				},
				{
					Key:         fieldPool,
					Label:       "pool",
					Placeholder: "none",
					Help:        "proxmox resource pool the vms join — blank for none",
					ConfigSet:   proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Pool = v }),
					ConfigGet:   proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Pool }),
				},
				{
					Key:       fieldHA,
					Label:     "ha",
					Default:   valNo,
					Help:      "register the vms with ha-manager so a failed node's vms restart elsewhere",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: wizard.SetBool(func(c *config.Config, v bool) { c.Provider.Proxmox.HA = v }),
					ConfigGet: getBool(func(c *config.Config) bool { return c.Provider.Proxmox.HA }),
				},
				{
					Key:       config.FieldProxmoxVMIDBase,
					Label:     labelVMIDBase,
					Default:   "6000",
					Help:      "first vm id — masters take base+10..12 and workers base+100 onward, e.g. 6000",
					Width:     wizard.FieldWidthNumber,
					Required:  true,
					Validate:  minInt(100, ""),
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Provider.Proxmox.VMIDBase = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Provider.Proxmox.VMIDBase }),
				},
			},
		},
	},
	Apply: func(_ *wizard.DataDrivenStep, cfg *config.Config) error {
		cfg.Provider.Type = config.ProviderProxmox
		return nil
	},
	Secrets: func(s *wizard.DataDrivenStep) wizard.Secrets {
		var secrets wizard.Secrets
		secrets.ProxmoxToken.Set(s.Value(fieldAPIToken))
		secrets.ProxmoxInsecure = s.Value(fieldInsecure) == valYes
		return secrets
	},
}

// NewProxmoxStep returns the Proxmox wizard step.
func NewProxmoxStep() *wizard.DataDrivenStep {
	return wizard.NewDataDrivenStep(&ProxmoxStepDefinition)
}
