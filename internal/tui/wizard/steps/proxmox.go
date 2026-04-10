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
	ID:           wizard.StepIDProxmox,
	Title:        "proxmox configuration",
	DisplayTitle: "configure proxmox ve connection",
	Description:  "configure proxmox connection",
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
				{
					Key:     "skip_tls_verify",
					Label:   "skip tls verify",
					Default: valNo,
					Help:    "skip tls certificate verification — set to yes only for self-signed certs",
					Type:    wizard.FieldTypeSelect,
					Options: []string{valNo, valYes},
					ConfigSet: wizard.SetBool(func(c *config.Config, v bool) {
						if c.Provider.Proxmox != nil {
							c.Provider.Proxmox.Insecure = v
						}
					}),
					ConfigGet: func(c *config.Config) string {
						if c.Provider.Proxmox != nil && c.Provider.Proxmox.Insecure {
							return valYes
						}
						return valNo
					},
				},
			},
		},
	},
	ShouldShow: func(cfg *config.Config) bool {
		return cfg.Provider.Type == config.ProviderProxmox
	},
}

func NewProxmoxStep() (*wizard.DataDrivenStep, *wizard.DataDrivenStep) {
	step := wizard.NewDataDrivenStep(&ProxmoxStepDefinition)
	return step, step
}
