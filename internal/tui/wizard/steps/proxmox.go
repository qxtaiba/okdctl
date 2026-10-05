package steps

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// pairKeyCredentials joins username and password into one paired row;
// named apart from the "credentials" section title since the two strings
// serve different purposes and a shared literal would read as accidental.
const pairKeyCredentials = "username_password"

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

// ProxmoxStepDefinition declares the Proxmox connection step fields.
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
					Key:         fieldHost,
					Label:       fieldHost,
					Default:     "192.168.1.100:8006",
					Placeholder: "192.168.1.100:8006",
					Help:        "proxmox host ip:port (e.g., 192.168.1.100:8006)",
					Required:    true,
					Validate:    config.ValidateProxmoxHost,
					ConfigSet:   proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Host = v }),
					ConfigGet:   proxmoxGet(func(p *config.ProxmoxConfig) string { return p.Host }),
				},
			},
		},
		{
			Title: "credentials",
			Fields: []wizard.FieldDefinition{
				{
					Key:       fieldUsername,
					Label:     fieldUsername,
					Default:   "root@pam",
					Help:      "proxmox username (user@realm)",
					Required:  true,
					PairKey:   pairKeyCredentials,
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
					PairKey:   pairKeyCredentials,
					ConfigSet: proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.Password.Set(v) }),
					// Don't load password from config
				},
			},
		},
		{
			Title:       "advanced",
			Collapsible: true,
			// Neither field here is Required nor can ever fail Check (a
			// select with a default can't error), so the fold's collapse
			// is accidentally safe today — isComplete() still forces it
			// open the moment either changes, which is the structural
			// guarantee the HARD CONSTRAINT needs regardless.
			FoldSummary: func(values map[string]string) []tui.FactRow {
				tokenStatus := "not set"
				if values["token_id"] != "" {
					tokenStatus = "set"
				}
				verification := "enabled"
				if values["skip_tls_verify"] == valYes {
					verification = "disabled"
				}
				return []tui.FactRow{
					{Key: labelTokenID, Value: tokenStatus},
					{Key: "verification", Value: verification},
				}
			},
			Fields: []wizard.FieldDefinition{
				{
					Key:         "token_id",
					Label:       labelTokenID,
					Default:     "",
					Placeholder: "user@pve!okdctl",
					Help:        "api token id (user@realm!tokenname) — saved to config; combined with the token secret at deploy time via PROXMOX_VE_API_TOKEN in id=secret form",
					ConfigSet:   proxmoxSet(func(p *config.ProxmoxConfig, v string) { p.TokenID = v }),
					ConfigGet:   proxmoxGet(func(p *config.ProxmoxConfig) string { return p.TokenID }),
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
	Apply: func(_ *wizard.DataDrivenStep, cfg *config.Config) error {
		cfg.Provider.Type = config.ProviderProxmox
		return nil
	},

	// Answered echoes only host and username: password and token_id are
	// both credential material and must never reach the context pane.
	Answered: func(values map[string]string) []render.Fact {
		var facts []render.Fact
		if v := values[fieldHost]; v != "" {
			facts = append(facts, render.Fact{Key: "host", Value: v})
		}
		if v := values[fieldUsername]; v != "" {
			facts = append(facts, render.Fact{Key: fieldUsername, Value: v})
		}
		return facts
	},
}

// NewProxmoxStep returns the Proxmox wizard step.
func NewProxmoxStep() *wizard.DataDrivenStep {
	return wizard.NewDataDrivenStep(&ProxmoxStepDefinition)
}
