package tui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
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

// TestThemeRemapReachesRenderedRows pins bug 14: the light-background and
// high-contrast remaps must reach a rendered dottedKV row and CodeInline —
// not stop at the eight aliases — or credentials and recovery commands
// render at ~2.4:1 on light terminals with the a11y switch on.
func TestThemeRemapReachesRenderedRows(t *testing.T) {
	forced := colorprofile.TrueColor
	outputProfile.Store(&forced)
	t.Cleanup(func() {
		SetColorProfileFor(&bytes.Buffer{})
		setTheme(ThemeDefault)
		SetDarkBackground(true)
	})

	SetDarkBackground(false)
	row := DottedKeyValueFull("console", "https://example", 12, 60)
	if strings.Contains(row, "148;163;184") {
		t.Fatalf("light dottedKV key still renders Slate400: %q", row)
	}
	if !strings.Contains(row, "71;85;105") {
		t.Fatalf("light dottedKV key does not carry the Slate600 remap: %q", row)
	}
	if code := CodeInlineStyle.Render("kubeadmin-password"); strings.Contains(code, "34;211;238") {
		t.Fatalf("light CodeInline still renders Cyan400: %q", code)
	}
	// The faint tier re-tunes alongside its siblings: a nested key renders
	// one muted tier below the light dim (Slate500), never the dim tier
	// itself and never nothing.
	sub := DottedKeyValueSubFull("password", "hunter2-placeholder", 12, 60)
	if !strings.Contains(sub, "100;116;139") {
		t.Fatalf("light nested key does not carry the Slate500 faint tier: %q", sub)
	}

	setTheme(ThemeHighContrast)
	rebuildStyles()
	if code := CodeInlineStyle.Render("kubeadmin-password"); strings.Contains(code, "34;211;238") {
		t.Fatalf("high-contrast CodeInline still renders Cyan400: %q", code)
	}
	row = DottedKeyValueFull("console", "https://example", 12, 60)
	if strings.Contains(row, "148;163;184") || strings.Contains(row, "71;85;105") {
		t.Fatalf("high-contrast dottedKV key still renders a slate tier: %q", row)
	}
	sub = DottedKeyValueSubFull("password", "hunter2-placeholder", 12, 60)
	if strings.Contains(sub, "100;116;139") {
		t.Fatalf("high-contrast nested key still renders the slate faint tier: %q", sub)
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
