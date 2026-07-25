package steps

import (
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

const cpuTypeHost = "host"

// AdvancedStepDefinition declares the advanced-settings step fields.
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
					Default: cpuTypeHost,
					Help:    "host gives best performance, x86-64-v2 or kvm64 allow live migration",
					Type:    wizard.FieldTypeSelect,
					Options: []string{cpuTypeHost, "x86-64-v2", "x86-64-v3", "kvm64"},
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
						return cpuTypeHost
					},
				},
				{
					Key:     "numa_enabled",
					Label:   "enable numa",
					Default: valNo,
					Help:    "enable numa topology for vms — improves performance on multi-socket hosts",
					Type:    wizard.FieldTypeSelect,
					Options: []string{valNo, valYes},
					ConfigSet: func(cfg *config.Config, value string) error {
						if cfg.Provider.Proxmox != nil {
							cfg.Provider.Proxmox.NUMAEnabled = value == valYes
						}
						return nil
					},
					ConfigGet: func(cfg *config.Config) string {
						if cfg.Provider.Proxmox != nil && cfg.Provider.Proxmox.NUMAEnabled {
							return valYes
						}
						return valNo
					},
				},
				{
					Key:     "ha_enabled",
					Label:   "enable ha anti-affinity",
					Default: valNo,
					Help:    "spread master vms across proxmox nodes via ha-manager anti-affinity — requires pve 9+ AND a multi-node pve cluster (a single host cannot satisfy the anti-affinity rule); once enabled, ha supersedes the per-vm startup{} ordering on any ha-triggered relocation",
					Type:    wizard.FieldTypeSelect,
					Options: []string{valNo, valYes},
					ConfigSet: wizard.SetBool(func(c *config.Config, v bool) {
						if c.Provider.Proxmox != nil {
							c.Provider.Proxmox.HAEnabled = v
						}
					}),
					ConfigGet: func(cfg *config.Config) string {
						if cfg.Provider.Proxmox != nil && cfg.Provider.Proxmox.HAEnabled {
							return valYes
						}
						return valNo
					},
				},
			},
		},
		{
			Title: "clock synchronization",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "ntp_server",
					Label:     "ntp server",
					Default:   "",
					Help:      "chrony source for master/worker nodes — blank uses the bastion's ignition server ip",
					Validate:  config.ValidateNTPServer,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Networking.NTPServer = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Networking.NTPServer }),
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
		{
			Title: "deployment options",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "terraform_env",
					Label:     "terraform environment",
					Default:   "",
					Help:      "selects a directory under infrastructure/terraform/environments/ — leave blank to use the default (production)",
					Validate:  config.ValidateTerraformEnv,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Deployment.TerraformEnv = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Deployment.TerraformEnv }),
				},
				{
					Key:       "auto_approve",
					Label:     "auto approve",
					Default:   valNo,
					Help:      "skip terraform apply confirmation prompts — use with care",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: wizard.SetBool(func(c *config.Config, v bool) { c.Deployment.AutoApprove = v }),
					ConfigGet: func(c *config.Config) string {
						if c.Deployment.AutoApprove {
							return valYes
						}
						return valNo
					},
				},
				{
					Key:       "bin_dir",
					Label:     "bin dir",
					Default:   "",
					Help:      "absolute path to install oc, openshift-install and tools; ~/ is expanded. blank = /usr/local/bin",
					Validate:  config.ValidateBinDir,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Deployment.BinDir = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Deployment.BinDir }),
				},
			},
		},
	},
	ExtraContent: func(_ map[string]string, _ int) string {
		subtitle := lipgloss.NewStyle().
			Foreground(tui.ColorSlate400).
			Italic(true).
			Render("these settings have sensible defaults - adjust only if needed")
		return subtitle
	},
}

// NewAdvancedStep returns the advanced-settings wizard step.
func NewAdvancedStep() *wizard.DataDrivenStep {
	return wizard.NewDataDrivenStep(&AdvancedStepDefinition)
}
