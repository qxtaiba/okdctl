package steps

import (
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
)

func proxmoxGet(getter func(p *config.ProxmoxConfig) string) wizard.ConfigGetter {
	return func(cfg *config.Config) string {
		if cfg.Provider.Proxmox == nil {
			return ""
		}
		return getter(cfg.Provider.Proxmox)
	}
}

func proxmoxSet(setter func(p *config.ProxmoxConfig, v string)) wizard.ConfigSetter {
	return func(cfg *config.Config, value string) error {
		if cfg.Provider.Proxmox == nil {
			cfg.Provider.Proxmox = &config.ProxmoxConfig{}
		}
		setter(cfg.Provider.Proxmox, value)
		return nil
	}
}

var ProxmoxStepDefinition = wizard.StepDefinition{
	ID:           "proxmox",
	Title:        "proxmox configuration",
	DisplayTitle: "configure proxmox ve connection",
	Description:  "configure proxmox connection and storage",
	Sections: []wizard.SectionDefinition{
		{
			Title: "connection",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "host",
					Label:     "host",
					Default:   "192.168.1.100:8006",
					Help:      "proxmox host ip:port (e.g., 192.168.1.100:8006)",
					Required:  true,
					Validate:  ValidateProxmoxHost,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Host = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Host }),
				},
				{
					Key:       "node",
					Label:     "node",
					Default:   "pve",
					Help:      "proxmox node name",
					Required:  true,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Node = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Node }),
				},
				{
					Key:       "bridge",
					Label:     "bridge",
					Default:   "vmbr0",
					Help:      "network bridge for vms",
					Required:  true,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Bridge = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Bridge }),
				},
			},
		},
		{
			Title: "storage",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "os_storage",
					Label:     "os storage",
					Default:   "local-lvm",
					Help:      "storage pool for vm boot disks",
					Required:  true,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Storage = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Storage }),
				},
				{
					Key:       "data_storage",
					Label:     "data storage",
					Default:   "local-lvm",
					Help:      "storage pool for data disks (optional)",
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.DataStorage = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.DataStorage }),
				},
				{
					Key:       "iso_storage",
					Label:     "iso storage",
					Default:   "local",
					Help:      "storage for iso files",
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.ISOStorage = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.ISOStorage }),
				},
			},
		},
		{
			Title: "credentials",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "username",
					Label:     "username",
					Default:   "root@pam",
					Help:      "proxmox username (user@realm)",
					Required:  true,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Username = v }),
					ConfigGet: proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Username }),
				},
				{
					Key:       "password",
					Label:     "password",
					Default:   "",
					Help:      "proxmox password",
					Type:      wizard.FieldTypePassword,
					Required:  true,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Password = v }),
					// Don't load password from config
				},
			},
		},
	},
	ShouldShow: func(cfg *config.Config) bool {
		return cfg.Provider.Type == config.ProviderProxmox
	},
	Apply: func(step *wizard.DataDrivenStep, cfg *config.Config) error {
		if cfg.Provider.Proxmox != nil {
			cfg.Provider.Proxmox.Insecure = true
		}
		return nil
	},
}

func NewProxmoxStep() (*wizard.DataDrivenStep, *wizard.DataDrivenStep) {
	step := wizard.NewDataDrivenStep(ProxmoxStepDefinition)
	return step, step
}
