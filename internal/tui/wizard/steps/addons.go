// Package steps defines the individual wizard steps (welcome, basics,
// distribution, proxmox, networking, resources, addons, advanced, files,
// review) that collect and validate a deployment config.
package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
)

const (
	valYes = "yes"
	valNo  = "no"
)

func addonEnabled(name string) wizard.ConfigGetter {
	return func(c *config.Config) string {
		if ac, ok := c.Addons[name]; ok && ac.Enabled {
			return valYes
		}
		return valNo
	}
}

func setAddonEnabled(name string) wizard.ConfigSetter {
	return wizard.SetBool(func(c *config.Config, v bool) {
		if c.Addons == nil {
			c.Addons = make(map[string]config.AddonConfig)
		}
		ac := c.Addons[name]
		ac.Enabled = v
		c.Addons[name] = ac
	})
}

func addonSetting(name, key string) wizard.ConfigGetter {
	return func(c *config.Config) string {
		if ac, ok := c.Addons[name]; ok && ac.Settings != nil {
			return ac.Settings[key]
		}
		return ""
	}
}

func setAddonSetting(name, key string) wizard.ConfigSetter {
	return wizard.SetString(func(c *config.Config, v string) {
		if c.Addons == nil {
			c.Addons = make(map[string]config.AddonConfig)
		}
		ac := c.Addons[name]
		if ac.Settings == nil {
			ac.Settings = make(map[string]string)
		}
		ac.Settings[key] = v
		c.Addons[name] = ac
	})
}

var AddonsStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDAddons,
	Title:        "cluster addons",
	DisplayTitle: "configure cluster addons",
	Description:  "enable optional cluster features",
	Sections: []wizard.SectionDefinition{
		{
			Title: "gitops (flux)",
			Note:  "requires: ssh deploy key at ~/.ssh/flux-deploy-key",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "flux_enabled",
					Label:     "enabled",
					Default:   "no",
					Help:      "enable gitops deployment",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: setAddonEnabled("flux"),
					ConfigGet: addonEnabled("flux"),
				},
				{
					Key:       "flux_repository",
					Label:     "repository",
					Default:   "",
					Help:      "git repository url (e.g., ssh://git@github.com/org/repo.git)",
					ConfigSet: setAddonSetting("flux", "repository"),
					ConfigGet: addonSetting("flux", "repository"),
				},
				{
					Key:       "flux_branch",
					Label:     "branch",
					Default:   "main",
					Help:      "branch to sync (e.g., main, develop)",
					ConfigSet: setAddonSetting("flux", "branch"),
					ConfigGet: addonSetting("flux", "branch"),
				},
				{
					Key:       "flux_path",
					Label:     "path",
					Default:   "kubernetes/clusters/production",
					Help:      "path within repository for manifests",
					ConfigSet: setAddonSetting("flux", "path"),
					ConfigGet: addonSetting("flux", "path"),
				},
			},
		},
		{
			Title: "1password secret store",
			Note:  "requires: sops-encrypted 1password-credentials.json and 1password-token.txt + age key on bastion",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_enabled",
					Label:     "enabled",
					Default:   "no",
					Help:      "bootstrap 1password connect secrets from sops files",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: setAddonEnabled("secretstore"),
					ConfigGet: addonEnabled("secretstore"),
				},
				{
					Key:       "secretstore_secrets_dir",
					Label:     "secrets directory",
					Default:   "automation/config/secrets",
					Help:      "path to sops-encrypted files (relative to project root)",
					ConfigSet: setAddonSetting("secretstore", "secrets_dir"),
					ConfigGet: addonSetting("secretstore", "secrets_dir"),
				},
			},
		},
	},
}

func NewAddonsStep() (step, state *wizard.DataDrivenStep) {
	step = wizard.NewDataDrivenStep(&AddonsStepDefinition)
	step.WithExtraContentFunc(func(s *wizard.DataDrivenStep, _ int) string {
		return renderAddonWarnings(s)
	})
	return step, step
}

func renderAddonWarnings(step *wizard.DataDrivenStep) string {
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	var warnings []string

	if step.Value("flux_enabled") == valYes {
		home, _ := os.UserHomeDir()
		keyPath := filepath.Join(home, ".ssh", "flux-deploy-key")
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			warnings = append(warnings, warnStyle.Render("  flux requires ssh deploy key at ~/.ssh/flux-deploy-key — create it before deploying"))
		}
	}

	if step.Value("secretstore_enabled") == valYes {
		if _, err := exec.LookPath("sops"); err != nil {
			warnings = append(warnings, warnStyle.Render("  secretstore requires sops — install before deploying"))
		}
	}

	if len(warnings) == 0 {
		return ""
	}
	return "\n" + strings.Join(warnings, "\n")
}
