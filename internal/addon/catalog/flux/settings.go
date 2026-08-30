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
	// AcceptHostKey opts into TOFU when GitHostFingerprint is empty; otherwise fails closed.
	AcceptHostKey     bool
	ControllerTimeout time.Duration
	GitSyncTimeout    time.Duration
}

// decodeSettings errors only on a non-positive-integer controller_timeout or git_sync_timeout.
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

// parseTimeoutSetting falls back only when unset; a malformed present value is an error.
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
