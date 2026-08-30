package tui

import "testing"

// TestHighContrastRequested pins the env contract after the legacy
// HOMELAB_HIGH_CONTRAST alias was removed: only OKDCTL_HIGH_CONTRAST set to
// "1" or "true" requests the high-contrast theme.
func TestHighContrastRequested(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"0", false},
		{"", false},
		{"yes", false},
	} {
		t.Setenv("OKDCTL_HIGH_CONTRAST", tc.value)
		if got := highContrastRequested(); got != tc.want {
			t.Errorf("OKDCTL_HIGH_CONTRAST=%q: highContrastRequested() = %v, want %v", tc.value, got, tc.want)
		}
	}
}
