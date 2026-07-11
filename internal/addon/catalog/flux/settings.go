package flux

// Settings is the typed representation of the flux addon's settings map.
type Settings struct {
	Repository         string
	Branch             string
	Path               string
	GitHostFingerprint string
	// AcceptHostKey opts into TOFU when GitHostFingerprint is empty. Without
	// either field set, createDeployKeySecret fails closed.
	AcceptHostKey bool
}

// decodeSettings converts the flat settings map into a Settings value.
// Install and ValidateSettings call this directly instead of the exported
// DecodeSettings so neither needs an unchecked type assertion on any.
func (f *Flux) decodeSettings(settings map[string]string) Settings {
	return Settings{
		Repository:         settings[SettingRepository],
		Branch:             orDefault(settings[SettingBranch], defaultBranch),
		Path:               orDefault(settings[SettingPath], defaultPath),
		GitHostFingerprint: settings[SettingGitHostFingerprint],
		AcceptHostKey:      settings[SettingAcceptHostKey] == "true",
	}
}

// DecodeSettings satisfies addon.ConfigurableAddon; see decodeSettings for
// the typed path used internally by this package.
func (f *Flux) DecodeSettings(settings map[string]string) (any, error) {
	return f.decodeSettings(settings), nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
