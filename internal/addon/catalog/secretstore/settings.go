package secretstore

// Settings is the typed representation of the secretstore addon's
// settings map. Exactly one of OnePassword, Vault, or Bitwarden is non-nil
// after a successful DecodeSettings call, matching the active Provider.
type Settings struct {
	Provider    providerKind
	SecretsDir  string
	OnePassword *onePasswordSettings
	Vault       *vaultSettings
	Bitwarden   *bitwardenSettings
}

// onePasswordSettings holds decoded settings for the onepassword provider.
type onePasswordSettings struct {
	ConnectHost string
	Vaults      map[string]int
}

// vaultSettings holds decoded settings for the vault provider.
type vaultSettings struct {
	Server  string
	Path    string
	Version string
}

// bitwardenSettings holds decoded settings for the bitwarden provider.
type bitwardenSettings struct {
	OrganizationID string
	ProjectID      string
	APIURL         string
	IdentityURL    string
	SDKServerURL   string
}

// decodeSettings converts the flat settings map into a Settings value,
// populating only the sub-struct for the active provider. An error is
// returned only when the onepassword vault CSV is malformed. Install and
// ValidateSettings call this directly instead of the exported
// DecodeSettings so neither needs an unchecked type assertion on any.
func (s *secretStore) decodeSettings(settings map[string]string) (Settings, error) {
	prov := providerKind(settings[SettingProvider])
	if prov == "" {
		prov = providerOnepassword
	}
	ts := Settings{
		Provider:   prov,
		SecretsDir: settingOrDefault(settings, SettingSecretsDir, defaultSecretsDir),
	}
	switch prov {
	case providerOnepassword:
		vaults, err := parseOnepasswordVaults(settings[SettingOnepasswordVaults])
		if err != nil {
			return Settings{}, err
		}
		ts.OnePassword = &onePasswordSettings{
			ConnectHost: settingOrDefault(settings, SettingOnepasswordConnectHost, defaultOPConnectHost),
			Vaults:      vaults,
		}
	case providerVault:
		ts.Vault = &vaultSettings{
			Server:  settings[SettingVaultServer],
			Path:    settingOrDefault(settings, SettingVaultPath, "secret"),
			Version: settingOrDefault(settings, SettingVaultVersion, "v2"),
		}
	case providerBitwarden:
		ts.Bitwarden = &bitwardenSettings{
			OrganizationID: settings[SettingBitwardenOrganizationID],
			ProjectID:      settings[SettingBitwardenProjectID],
			APIURL:         settingOrDefault(settings, SettingBitwardenAPIURL, defaultBitwardenAPIURL),
			IdentityURL:    settingOrDefault(settings, SettingBitwardenIdentityURL, defaultBitwardenIdentityURL),
			SDKServerURL:   settingOrDefault(settings, SettingBitwardenSDKServerURL, defaultBitwardenSDKServerURL),
		}
	}
	return ts, nil
}

// DecodeSettings satisfies addon.ConfigurableAddon; see decodeSettings for
// the typed path used internally by this package.
func (s *secretStore) DecodeSettings(settings map[string]string) (any, error) {
	return s.decodeSettings(settings)
}
