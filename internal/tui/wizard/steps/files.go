package steps

import (
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// FilesStepDefinition declares the pull-secret and SSH-key fields.
var FilesStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDFiles,
	Title:        "files",
	DisplayTitle: "configure required files",
	Description:  "point at the pull secret and ssh key baked into the iso",
	Sections: []wizard.SectionDefinition{
		{
			Title: "required files",
			Fields: []wizard.FieldDefinition{
				{
					Key:         config.FieldFilesPullSecret,
					Label:       "pull secret",
					Placeholder: "~/pull-secret.json",
					Help:        "path to the okd pull secret json from red hat",
					Width:       wizard.FieldWidthPath,
					Required:    true,
					Validate:    ValidateFilePath,
					ConfigSet:   wizard.SetString(func(c *config.Config, v string) { c.Files.PullSecret = v }),
					ConfigGet:   wizard.GetString(func(c *config.Config) string { return c.Files.PullSecret }),
				},
				{
					Key:         config.FieldFilesSSHPublicKey,
					Label:       "ssh public key",
					Placeholder: "~/.ssh/id_ed25519.pub",
					Help:        "path to the ssh public key for node access",
					Width:       wizard.FieldWidthPath,
					Required:    true,
					Validate:    ValidateFilePath,
					ConfigSet:   wizard.SetString(func(c *config.Config, v string) { c.Files.SSHPublicKey = v }),
					ConfigGet:   wizard.GetString(func(c *config.Config) string { return c.Files.SSHPublicKey }),
				},
			},
		},
	},
	ExtraContentTitle: "notes",
	ExtraContent: func(_ map[string]string, _ int) string {
		return lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Italic(true).
			Render("pull secret: get from https://console.redhat.com/openshift/install/pull-secret")
	},
}

// NewFilesStep returns the files wizard step.
func NewFilesStep() *wizard.DataDrivenStep {
	return wizard.NewDataDrivenStep(&FilesStepDefinition)
}
