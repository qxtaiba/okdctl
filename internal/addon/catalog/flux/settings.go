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

// DecodeSettings converts the flat settings map into a Settings value.
func (f *Flux) DecodeSettings(settings map[string]string) (any, error) {
	return Settings{
		Repository:         settings[SettingRepository],
		Branch:             orDefault(settings[SettingBranch], "main"),
		Path:               orDefault(settings[SettingPath], "kubernetes/clusters/production"),
		GitHostFingerprint: settings[SettingGitHostFingerprint],
		AcceptHostKey:      settings[SettingAcceptHostKey] == "true",
	}, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
