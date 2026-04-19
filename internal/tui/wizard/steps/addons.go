// Package steps defines the individual wizard steps (welcome, basics,
// distribution, proxmox, networking, resources, addons, advanced, files,
// review) that collect and validate a deployment config.
package steps

import (
	"os/exec"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/addon/catalog/flux"
	"github.com/qxtaiba/okdctl/internal/addon/catalog/secretstore"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
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

// AddonsStepDefinition declares the cluster-addons step fields.
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
					ConfigSet: setAddonSetting("flux", flux.SettingRepository),
					ConfigGet: addonSetting("flux", flux.SettingRepository),
				},
				{
					Key:       "flux_branch",
					Label:     "branch",
					Default:   "main",
					Help:      "branch to sync (e.g., main, develop)",
					ConfigSet: setAddonSetting("flux", flux.SettingBranch),
					ConfigGet: addonSetting("flux", flux.SettingBranch),
				},
				{
					Key:       "flux_path",
					Label:     "path",
					Default:   "kubernetes/clusters/production",
					Help:      "path within repository for manifests",
					ConfigSet: setAddonSetting("flux", flux.SettingPath),
					ConfigGet: addonSetting("flux", flux.SettingPath),
				},
			},
		},
		{
			Title: "secret store (common)",
			Note:  "supports onepassword, vault, and bitwarden via external-secrets-operator",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_enabled",
					Label:     "enabled",
					Default:   "no",
					Help:      "bootstrap ESO provider credentials and SecretStore CRD",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: setAddonEnabled("secretstore"),
					ConfigGet: addonEnabled("secretstore"),
				},
				{
					Key:       "secretstore_provider",
					Label:     "provider",
					Default:   "onepassword",
					Help:      "ESO backend: onepassword, vault, bitwarden",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{"onepassword", "vault", "bitwarden"},
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingProvider),
					ConfigGet: addonSetting("secretstore", secretstore.SettingProvider),
				},
				{
					Key:       "secretstore_secrets_dir",
					Label:     "secrets directory",
					Default:   "automation/config/secrets",
					Help:      "path to provider credential files (relative to project root)",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingSecretsDir),
					ConfigGet: addonSetting("secretstore", secretstore.SettingSecretsDir),
				},
			},
		},
		{
			Title: "secret store (onepassword)",
			Note:  "requires: sops-encrypted 1password-credentials.json and 1password-token.txt + age key on bastion",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_op_connect_host",
					Label:     "connect host",
					Default:   "http://onepassword-connect:8080",
					Help:      "1Password Connect server URL",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingOnepasswordConnectHost),
					ConfigGet: addonSetting("secretstore", secretstore.SettingOnepasswordConnectHost),
				},
				{
					Key:       "secretstore_op_vaults",
					Label:     "vaults",
					Default:   "homelab=1",
					Help:      "CSV of name=priority pairs, e.g. \"homelab=1,shared=2\"",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingOnepasswordVaults),
					ConfigGet: addonSetting("secretstore", secretstore.SettingOnepasswordVaults),
				},
			},
		},
		{
			Title: "secret store (vault)",
			Note:  "requires: vault-token.txt in secrets directory (plaintext or sops-encrypted)",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_vault_server",
					Label:     "server url",
					Default:   "",
					Help:      "Vault server URL (e.g. https://vault.example.com)",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingVaultServer),
					ConfigGet: addonSetting("secretstore", secretstore.SettingVaultServer),
				},
				{
					Key:       "secretstore_vault_path",
					Label:     "secret path",
					Default:   "secret",
					Help:      "Vault KV mount path",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingVaultPath),
					ConfigGet: addonSetting("secretstore", secretstore.SettingVaultPath),
				},
				{
					Key:       "secretstore_vault_version",
					Label:     "kv version",
					Default:   "v2",
					Help:      "Vault KV engine version: v1 or v2",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingVaultVersion),
					ConfigGet: addonSetting("secretstore", secretstore.SettingVaultVersion),
				},
			},
		},
		{
			Title: "secret store (bitwarden)",
			Note:  "requires: bitwarden-token.txt in secrets directory (plaintext or sops-encrypted)",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_bw_org_id",
					Label:     "organization id",
					Default:   "",
					Help:      "Bitwarden organization UUID",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenOrganizationID),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenOrganizationID),
				},
				{
					Key:       "secretstore_bw_project_id",
					Label:     "project id",
					Default:   "",
					Help:      "Bitwarden project UUID",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenProjectID),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenProjectID),
				},
				{
					Key:       "secretstore_bw_api_url",
					Label:     "api url",
					Default:   "https://api.bitwarden.com",
					Help:      "Bitwarden Secrets Manager API URL",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenAPIURL),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenAPIURL),
				},
				{
					Key:       "secretstore_bw_identity_url",
					Label:     "identity url",
					Default:   "https://identity.bitwarden.com",
					Help:      "Bitwarden identity service URL",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenIdentityURL),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenIdentityURL),
				},
				{
					Key:       "secretstore_bw_sdk_url",
					Label:     "sdk server url",
					Default:   "https://bitwarden-sdk-server.external-secrets.svc.cluster.local:9998",
					Help:      "in-cluster bitwarden-sdk-server URL",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenSDKServerURL),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenSDKServerURL),
				},
			},
		},
	},
}

// NewAddonsStep returns the addons wizard step and its state.
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
		keyPath := system.ExpandPath("~/.ssh/flux-deploy-key")
		if !system.FileExists(keyPath) {
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
