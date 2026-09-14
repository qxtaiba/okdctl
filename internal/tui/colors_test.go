package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

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

// TestSetDarkBackgroundRebindsMutedTiers pins the light-background muted
// tier hex values and confirms SetDarkBackground(true) restores the dark
// defaults.
func TestSetDarkBackgroundRebindsMutedTiers(t *testing.T) {
	t.Cleanup(func() { SetDarkBackground(true) })

	SetDarkBackground(false)
	if IsDarkBackground() {
		t.Error("IsDarkBackground() = true after SetDarkBackground(false)")
	}
	if ColorTextDim != lipgloss.Color("#475569") {
		t.Errorf("light ColorTextDim = %v, want #475569", ColorTextDim)
	}
	if ColorSlate500 != lipgloss.Color("#64748B") {
		t.Errorf("light ColorSlate500 = %v, want #64748B", ColorSlate500)
	}
	if ColorSlate700 != lipgloss.Color("#CBD5E1") {
		t.Errorf("light ColorSlate700 = %v, want #CBD5E1", ColorSlate700)
	}

	SetDarkBackground(true)
	if !IsDarkBackground() {
		t.Error("IsDarkBackground() = false after SetDarkBackground(true)")
	}
	if ColorTextDim != lipgloss.Color("#94A3B8") {
		t.Errorf("dark ColorTextDim = %v, want #94A3B8", ColorTextDim)
	}
	if ColorSlate500 != lipgloss.Color("#64748B") {
		t.Errorf("dark ColorSlate500 = %v, want #64748B", ColorSlate500)
	}
	if ColorSlate700 != lipgloss.Color("#334155") {
		t.Errorf("dark ColorSlate700 = %v, want #334155", ColorSlate700)
	}
}

// TestHighContrastPropagatesToBaseStyles proves rebuildStyles fixes the
// init-ordering bug where TitleStyle used to capture ColorPrimary before
// setTheme could rebind it.
func TestHighContrastPropagatesToBaseStyles(t *testing.T) {
	t.Cleanup(func() {
		setTheme(ThemeDefault)
		rebuildStyles()
	})

	setTheme(ThemeHighContrast)
	rebuildStyles()

	if got := TitleStyle.GetForeground(); got != hcColorPrimary {
		t.Errorf("TitleStyle.GetForeground() = %v, want %v", got, hcColorPrimary)
	}
}

// TestTextStyleHasNoForeground confirms TextStyle renders plain body text
// with the terminal's default foreground instead of a forced colour.
func TestTextStyleHasNoForeground(t *testing.T) {
	if _, ok := TextStyle.GetForeground().(lipgloss.NoColor); !ok {
		t.Errorf("TextStyle.GetForeground() = %v, want lipgloss.NoColor", TextStyle.GetForeground())
	}
}
