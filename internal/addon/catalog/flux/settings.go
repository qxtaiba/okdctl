package flux

import "strings"

// Settings is the typed representation of the flux addon's settings map.
type Settings struct {
	Repository       string
	Branch           string
	Path             string
	KnownHostsSHA256 string
	AcceptHostKey    bool
}

// DecodeSettings converts the flat settings map into a Settings value.
func (f *Flux) DecodeSettings(settings map[string]string) (any, error) {
	return Settings{
		Repository:       settings[SettingRepository],
		Branch:           orDefault(settings[SettingBranch], "main"),
		Path:             orDefault(settings[SettingPath], "kubernetes/clusters/production"),
		KnownHostsSHA256: settings[SettingKnownHostsSHA256],
		AcceptHostKey:    strings.EqualFold(settings[SettingAcceptHostKey], "true"),
	}, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
