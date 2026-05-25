package secretstore

// Settings is the typed representation of the secretstore addon's
// settings map. Exactly one of OnePassword, Vault, or Bitwarden is non-nil
// after a successful DecodeSettings call, matching the active Provider.
type Settings struct {
	Provider    ProviderKind
	SecretsDir  string
	OnePassword *OnePasswordSettings
	Vault       *VaultSettings
	Bitwarden   *BitwardenSettings
}

// OnePasswordSettings holds decoded settings for the onepassword provider.
type OnePasswordSettings struct {
	ConnectHost string
	Vaults      map[string]int
}

// VaultSettings holds decoded settings for the vault provider.
type VaultSettings struct {
	Server  string
	Path    string
	Version string
}

// BitwardenSettings holds decoded settings for the bitwarden provider.
type BitwardenSettings struct {
	OrganizationID string
	ProjectID      string
	APIURL         string
	IdentityURL    string
	SDKServerURL   string
}

// DecodeSettings converts the flat settings map into a Settings value,
// populating only the sub-struct for the active provider. An error is returned
// only when the onepassword vault CSV is malformed.
func (s *SecretStore) DecodeSettings(settings map[string]string) (any, error) {
	prov := ProviderKind(settings[SettingProvider])
	if prov == "" {
		prov = ProviderOnepassword
	}
	ts := Settings{
		Provider:   prov,
		SecretsDir: settingOrDefault(settings, SettingSecretsDir, defaultSecretsDir),
	}
	switch prov {
	case ProviderOnepassword:
		vaults, err := parseOnepasswordVaults(settings[SettingOnepasswordVaults])
		if err != nil {
			return nil, err
		}
		ts.OnePassword = &OnePasswordSettings{
			ConnectHost: settingOrDefault(settings, SettingOnepasswordConnectHost, defaultOPConnectHost),
			Vaults:      vaults,
		}
	case ProviderVault:
		ts.Vault = &VaultSettings{
			Server:  settings[SettingVaultServer],
			Path:    settingOrDefault(settings, SettingVaultPath, "secret"),
			Version: settingOrDefault(settings, SettingVaultVersion, "v2"),
		}
	case ProviderBitwarden:
		ts.Bitwarden = &BitwardenSettings{
			OrganizationID: settings[SettingBitwardenOrganizationID],
			ProjectID:      settings[SettingBitwardenProjectID],
			APIURL:         settingOrDefault(settings, SettingBitwardenAPIURL, defaultBitwardenAPIURL),
			IdentityURL:    settingOrDefault(settings, SettingBitwardenIdentityURL, defaultBitwardenIdentityURL),
			SDKServerURL:   settingOrDefault(settings, SettingBitwardenSDKServerURL, defaultBitwardenSDKServerURL),
		}
	}
	return ts, nil
}
