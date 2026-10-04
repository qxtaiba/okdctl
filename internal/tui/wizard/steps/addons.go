// Package steps defines the individual wizard steps that collect and
// validate a deployment config.
package steps

import (
	"errors"
	"os/exec"
	"strings"

	"github.com/qxtaiba/okdctl/internal/addon/catalog/flux"
	"github.com/qxtaiba/okdctl/internal/addon/catalog/secretstore"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

const (
	valYes     = "yes"
	valEnabled = "enabled"
	valNo      = "no"

	// secretstore provider values, shared by the field's Default/Options and
	// each provider section's Visible gate below.
	providerOnepassword = "onepassword"
	providerVault       = "vault"
	providerBitwarden   = "bitwarden"
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
	// Validate rejects an enabled addon missing the endpoint its install
	// cannot run without, so the wizard fails here rather than the addon
	// manager an hour into the deploy; the flux repository and bitwarden ids
	// are field-level Required instead, live exactly while their sections
	// are unfolded.
	Validate: func(values map[string]string) error {
		if values["secretstore_enabled"] == valYes && values["secretstore_provider"] == providerVault &&
			strings.TrimSpace(values["secretstore_vault_server"]) == "" {
			return wizard.NewCrossFieldError(
				errors.New("vault server url is required — enter the vault address"),
				"secretstore_vault_server",
			)
		}
		return nil
	},
	Sections: []wizard.SectionDefinition{
		{
			Title: "gitops (flux)",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "flux_enabled",
					Label:     valEnabled,
					Default:   "no",
					Help:      "enable gitops deployment",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: setAddonEnabled("flux"),
					ConfigGet: addonEnabled("flux"),
				},
			},
		},
		{
			Title: "flux settings",
			Note:  "requires: ssh deploy key at ~/.ssh/flux-deploy-key",
			Visible: func(values map[string]string) bool {
				return values["flux_enabled"] == valYes
			},
			Warning: func(values map[string]string) string {
				if values["flux_enabled"] != valYes {
					return ""
				}
				if system.FileExists(system.ExpandPath("~/.ssh/flux-deploy-key")) {
					return ""
				}
				return "flux requires ssh deploy key at ~/.ssh/flux-deploy-key — create it before deploying"
			},
			Fields: []wizard.FieldDefinition{
				{
					Key:       "flux_repository",
					Label:     "repository",
					Default:   "",
					Help:      "git repository url (e.g., ssh://git@github.com/org/repo.git)",
					Width:     wizard.FieldWidthPath,
					Required:  true,
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
					Width:     wizard.FieldWidthPath,
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
					Label:     valEnabled,
					Default:   "no",
					Help:      "bootstrap eso provider credentials and secretstore crd",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{valNo, valYes},
					ConfigSet: setAddonEnabled("secretstore"),
					ConfigGet: addonEnabled("secretstore"),
				},
			},
		},
		{
			Title: "secret store settings",
			Visible: func(values map[string]string) bool {
				return values["secretstore_enabled"] == valYes
			},
			Warning: secretStoreSopsWarning(sopsOnPath),
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_provider",
					Label:     "provider",
					Default:   providerOnepassword,
					Help:      "eso backend: onepassword, vault, bitwarden",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{providerOnepassword, providerVault, providerBitwarden},
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
			Visible: func(values map[string]string) bool {
				return values["secretstore_enabled"] == valYes && values["secretstore_provider"] == providerOnepassword
			},
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_op_connect_host",
					Label:     "connect host",
					Default:   "http://onepassword-connect:8080",
					Help:      "1password connect server url",
					Width:     wizard.FieldWidthPath,
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingOnepasswordConnectHost),
					ConfigGet: addonSetting("secretstore", secretstore.SettingOnepasswordConnectHost),
				},
				{
					Key:       "secretstore_op_vaults",
					Label:     "vaults",
					Default:   "homelab=1",
					Help:      "name=priority pairs, e.g. homelab=1,shared=2",
					Type:      wizard.FieldTypeKeyValue,
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingOnepasswordVaults),
					ConfigGet: addonSetting("secretstore", secretstore.SettingOnepasswordVaults),
				},
			},
		},
		{
			Title: "secret store (vault)",
			Note:  "requires: vault-token.txt in secrets directory (plaintext or sops-encrypted)",
			Visible: func(values map[string]string) bool {
				return values["secretstore_enabled"] == valYes && values["secretstore_provider"] == providerVault
			},
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_vault_server",
					Label:     "server url",
					Default:   "",
					Help:      "vault server url (e.g. https://vault.example.com)",
					Width:     wizard.FieldWidthPath,
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingVaultServer),
					ConfigGet: addonSetting("secretstore", secretstore.SettingVaultServer),
				},
				{
					Key:       "secretstore_vault_path",
					Label:     "secret path",
					Default:   "secret",
					Help:      "vault kv mount path",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingVaultPath),
					ConfigGet: addonSetting("secretstore", secretstore.SettingVaultPath),
				},
				{
					Key:       "secretstore_vault_version",
					Label:     "kv version",
					Default:   "v2",
					Help:      "vault kv engine version: v1 or v2",
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingVaultVersion),
					ConfigGet: addonSetting("secretstore", secretstore.SettingVaultVersion),
				},
			},
		},
		{
			Title: "secret store (bitwarden)",
			Note:  "requires: bitwarden-token.txt in secrets directory (plaintext or sops-encrypted)",
			Visible: func(values map[string]string) bool {
				return values["secretstore_enabled"] == valYes && values["secretstore_provider"] == providerBitwarden
			},
			Fields: []wizard.FieldDefinition{
				{
					Key:       "secretstore_bw_org_id",
					Label:     "organization id",
					Default:   "",
					Help:      "bitwarden organization uuid",
					Required:  true,
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenOrganizationID),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenOrganizationID),
				},
				{
					Key:       "secretstore_bw_project_id",
					Label:     "project id",
					Default:   "",
					Help:      "bitwarden project uuid",
					Required:  true,
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenProjectID),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenProjectID),
				},
				{
					Key:       "secretstore_bw_api_url",
					Label:     "api url",
					Default:   "https://api.bitwarden.com",
					Help:      "bitwarden secrets manager api url",
					Width:     wizard.FieldWidthPath,
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenAPIURL),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenAPIURL),
				},
				{
					Key:       "secretstore_bw_identity_url",
					Label:     "identity url",
					Default:   "https://identity.bitwarden.com",
					Help:      "bitwarden identity service url",
					Width:     wizard.FieldWidthPath,
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenIdentityURL),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenIdentityURL),
				},
				{
					Key:       "secretstore_bw_sdk_url",
					Label:     "sdk server url",
					Default:   "https://bitwarden-sdk-server.external-secrets.svc.cluster.local:9998",
					Help:      "in-cluster bitwarden-sdk-server url",
					Width:     wizard.FieldWidthPath,
					ConfigSet: setAddonSetting("secretstore", secretstore.SettingBitwardenSDKServerURL),
					ConfigGet: addonSetting("secretstore", secretstore.SettingBitwardenSDKServerURL),
				},
			},
		},
	},
}

// sopsOnPath resolves whether the sops binary is reachable on PATH; a test
// overrides it to prove the render path never calls it directly.
var sopsOnPath = func() bool {
	_, err := exec.LookPath("sops")
	return err == nil
}

// secretStoreSopsWarning returns the secret-store-settings section's
// Warning, deferring the sops-on-PATH question to check.
func secretStoreSopsWarning(check func() bool) func(values map[string]string) string {
	return func(values map[string]string) string {
		if values["secretstore_enabled"] != valYes {
			return ""
		}
		if check() {
			return ""
		}
		return "secretstore requires sops — install before deploying"
	}
}

// NewAddonsStep returns the addons wizard step, resolving the secretstore
// section's sops-on-PATH check once here instead of on every render.
func NewAddonsStep() *wizard.DataDrivenStep {
	def := AddonsStepDefinition
	def.Sections = append([]wizard.SectionDefinition(nil), AddonsStepDefinition.Sections...)

	sopsFound := sopsOnPath()
	for i := range def.Sections {
		if def.Sections[i].Title != "secret store settings" {
			continue
		}
		def.Sections[i].Warning = secretStoreSopsWarning(func() bool { return sopsFound })
	}

	return wizard.NewDataDrivenStep(&def)
}
