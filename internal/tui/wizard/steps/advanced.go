package steps

import (
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
)

var AdvancedStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDAdvanced,
	Title:        "advanced settings",
	DisplayTitle: "advanced settings",
	Description:  "configure vm ids and timeouts",
	Sections: []wizard.SectionDefinition{
		{
			Title: "vm settings",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "vm_id_base",
					Label:     "vm id base",
					Default:   "6000",
					Help:      "starting vm id in proxmox (e.g., 6000, 6001, ...)",
					Required:  true,
					Validate:  config.ValidateVMID,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.VMIDBase = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.VMIDBase }),
				},
			},
		},
		{
			Title: "proxmox vm settings",
			Fields: []wizard.FieldDefinition{
				{
					Key:     "cpu_type",
					Label:   "cpu type",
					Default: "host",
					Help:    "host gives best performance, x86-64-v2 or kvm64 allow live migration",
					Type:    wizard.FieldTypeSelect,
					Options: []string{"host", "x86-64-v2", "x86-64-v3", "kvm64"},
					ConfigSet: func(cfg *config.Config, value string) error {
						if cfg.Provider.Proxmox != nil {
							cfg.Provider.Proxmox.CPUType = value
						}
						return nil
					},
					ConfigGet: func(cfg *config.Config) string {
						if cfg.Provider.Proxmox != nil && cfg.Provider.Proxmox.CPUType != "" {
							return cfg.Provider.Proxmox.CPUType
						}
						return "host"
					},
				},
				{
					Key:     "numa_enabled",
					Label:   "enable numa",
					Default: "no",
					Help:    "enable numa topology for vms — improves performance on multi-socket hosts",
					Type:    wizard.FieldTypeSelect,
					Options: []string{"no", "yes"},
					ConfigSet: func(cfg *config.Config, value string) error {
						if cfg.Provider.Proxmox != nil {
							cfg.Provider.Proxmox.NUMAEnabled = value == "yes"
						}
						return nil
					},
					ConfigGet: func(cfg *config.Config) string {
						if cfg.Provider.Proxmox != nil && cfg.Provider.Proxmox.NUMAEnabled {
							return "yes"
						}
						return "no"
					},
				},
			},
		},
		{
			Title: "installation timeouts",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "bootstrap_timeout",
					Label:     "bootstrap timeout",
					Default:   "3600",
					Help:      "seconds to wait for bootstrap (default: 1 hour)",
					Required:  true,
					Validate:  config.ValidateTimeout,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Deployment.BootstrapTimeout = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Deployment.BootstrapTimeout }),
				},
				{
					Key:       "install_timeout",
					Label:     "install timeout",
					Default:   "7200",
					Help:      "seconds to wait for install (default: 2 hours)",
					Required:  true,
					Validate:  config.ValidateTimeout,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Deployment.InstallTimeout = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Deployment.InstallTimeout }),
				},
			},
		},
	},
	ExtraContent: func(values map[string]string, width int) string {
		subtitle := lipgloss.NewStyle().
			Foreground(tui.ColorSlate400).
			Italic(true).
			Render("these settings have sensible defaults - adjust only if needed")
		return subtitle
	},
}

func NewAdvancedStep() (*wizard.DataDrivenStep, *wizard.DataDrivenStep) {
	step := wizard.NewDataDrivenStep(AdvancedStepDefinition)
	return step, step
}
