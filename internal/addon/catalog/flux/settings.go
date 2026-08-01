package flux

import (
	"fmt"
	"strconv"
	"time"
)

// Settings is the typed representation of the flux addon's settings map.
type Settings struct {
	Repository         string
	Branch             string
	Path               string
	GitHostFingerprint string
	// AcceptHostKey opts into TOFU when GitHostFingerprint is empty. Without
	// either field set, createDeployKeySecret fails closed.
	AcceptHostKey     bool
	ControllerTimeout time.Duration
	GitSyncTimeout    time.Duration
}

// decodeSettings converts the flat settings map into a Settings value.
// Install and ValidateSettings call this directly instead of the exported
// DecodeSettings so neither needs an unchecked type assertion on any. It
// errors only when controller_timeout or git_sync_timeout is set to a
// non-positive-integer string.
func (f *fluxAddon) decodeSettings(settings map[string]string) (Settings, error) {
	controllerTimeout, err := parseTimeoutSetting(settings, SettingControllerTimeout, defaultControllerTimeout)
	if err != nil {
		return Settings{}, err
	}
	gitSyncTimeout, err := parseTimeoutSetting(settings, SettingGitSyncTimeout, defaultGitRepoSyncTimeout)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Repository:         settings[SettingRepository],
		Branch:             orDefault(settings[SettingBranch], defaultBranch),
		Path:               orDefault(settings[SettingPath], defaultPath),
		GitHostFingerprint: settings[SettingGitHostFingerprint],
		AcceptHostKey:      settings[SettingAcceptHostKey] == "true",
		ControllerTimeout:  controllerTimeout,
		GitSyncTimeout:     gitSyncTimeout,
	}, nil
}

// DecodeSettings satisfies addon.ConfigurableAddon; see decodeSettings for
// the typed path used internally by this package.
func (f *fluxAddon) DecodeSettings(settings map[string]string) (any, error) {
	return f.decodeSettings(settings)
}

// parseTimeoutSetting reads a timeout setting (seconds) from settings,
// falling back to fallback when the key is unset. A present but malformed
// value is an error rather than a silent fallback to fallback.
func parseTimeoutSetting(settings map[string]string, key string, fallback time.Duration) (time.Duration, error) {
	v, ok := settings[key]
	if !ok || v == "" {
		return fallback, nil
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer number of seconds, got %q", key, v)
	}
	return time.Duration(secs) * time.Second, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
