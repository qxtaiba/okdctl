package steps

import (
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// FilesStepDefinition declares the files / ignition-server step fields.
var FilesStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDFiles,
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
	Apply: func(_ *wizard.DataDrivenStep, cfg *config.Config) error {
		cfg.HTTPServer.IgnitionServerIP = cfg.Networking.Bastion.IP
		return nil
	},
	ExtraContent: func(_ map[string]string, _ int) string {
		helpStyle := lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).
			Italic(true)

		return helpStyle.Render(
			"pull secret: get from https://cloud.redhat.com/openshift/install/pull-secret\n" +
				"ignition server runs on bastion — ip is derived from networking step.")
	},
}

// NewFilesStep returns the files / ignition wizard step.
func NewFilesStep() (step *wizard.DataDrivenStep, _ any) {
	step = wizard.NewDataDrivenStep(&FilesStepDefinition)
	return step, nil
}
