package secretstore

// Settings is the typed representation of the secretstore addon's settings map.
// Exactly one of OnePassword, Vault, or Bitwarden is non-nil after
// decodeSettings, matching Provider.
type Settings struct {
	Provider    providerKind
	OnePassword *onePasswordSettings
	Vault       *vaultSettings
	Bitwarden   *bitwardenSettings
}

type onePasswordSettings struct {
	ConnectHost string
	Vaults      map[string]int
}

type vaultSettings struct {
	Server  string
	Path    string
	Version string
}

type bitwardenSettings struct {
	OrganizationID string
	ProjectID      string
	APIURL         string
	IdentityURL    string
	SDKServerURL   string
}

// decodeSettings converts the settings map into Settings for the active
// provider; it errors only on a malformed onepassword vault CSV.
func (s *secretStore) decodeSettings(settings map[string]string) (Settings, error) {
	prov := providerKind(settings[SettingProvider])
	if prov == "" {
		prov = providerOnepassword
	}
	ts := Settings{Provider: prov}
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
