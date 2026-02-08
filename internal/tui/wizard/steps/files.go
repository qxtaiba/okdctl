package steps

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

var FilesStepDefinition = wizard.StepDefinition{
	ID:           "files",
	Title:        "files & ignition",
	DisplayTitle: "configure files & ignition server",
	Description:  "configure required files and ignition server",
	Sections: []wizard.SectionDefinition{
		{
			Title: "required files",
			Fields: []wizard.FieldDefinition{
				{
					Key:      "pull_secret",
					Label:    "pull secret",
					Default:  "~/pull-secret.json",
					Help:     "path to okd pull-secret.json from red hat",
					Required: true,
					Validate: ValidateFilePath,
					ConfigSet: func(cfg *config.Config, value string) error {
						cfg.Files.PullSecret = system.ExpandPath(value)
						return nil
					},
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Files.PullSecret }),
				},
				{
					Key:      "ssh_public_key",
					Label:    "ssh public key",
					Default:  "~/.ssh/id_ed25519.pub",
					Help:     "path to ssh public key for node access",
					Required: true,
					Validate: ValidateFilePath,
					ConfigSet: func(cfg *config.Config, value string) error {
						cfg.Files.SSHPublicKey = system.ExpandPath(value)
						return nil
					},
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Files.SSHPublicKey }),
				},
			},
		},
		{
			Title: "ignition server",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "ignition_ip",
					Label:     "ignition server ip",
					Default:   "192.168.1.20",
					Help:      "ip where ignition files will be served (usually bastion)",
					Required:  true,
					Validate:  config.ValidateIP,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.HTTPServer.IgnitionServerIP = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.HTTPServer.IgnitionServerIP }),
				},
				{
					Key:       "http_port",
					Label:     "http port",
					Default:   "8080",
					Help:      "port for http server serving ignition files",
					Required:  true,
					Validate:  config.ValidatePortNumber,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.HTTPServer.Port = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.HTTPServer.Port }),
				},
				{
					Key:       "web_root",
					Label:     "web root",
					Default:   "/var/www/html",
					Help:      "directory to serve ignition files from",
					Required:  true,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.HTTPServer.Root = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.HTTPServer.Root }),
				},
			},
		},
	},
	ShouldShow: func(cfg *config.Config) bool {
		return cfg.Distribution.Type == config.DistributionOKD
	},
	ExtraContent: func(values map[string]string, width int) string {
		helpStyle := lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).
			Italic(true)

		return helpStyle.Render(
			"pull secret: get from https://cloud.redhat.com/openshift/install/pull-secret\n" +
				"the ignition server hosts boot configuration files for okd nodes.")
	},
}

func NewFilesStep() (*wizard.DataDrivenStep, any) {
	step := wizard.NewDataDrivenStep(FilesStepDefinition)
	return step, nil
}
