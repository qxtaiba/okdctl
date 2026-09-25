package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

func TestResolveThemeDualPolarityTable(t *testing.T) {
	dark := ResolveTheme(colorprofile.TrueColor, true, ThemeDefault)
	light := ResolveTheme(colorprofile.TrueColor, false, ThemeDefault)

	for _, tc := range []struct {
		role        string
		dark, light interface{ RGBA() (r, g, b, a uint32) }
		wantDark    string
		wantLight   string
	}{
		{"primary", dark.Primary, light.Primary, "#9333EA", "#7E22CE"},
		{"primary_dim", dark.PrimaryDim, light.PrimaryDim, "#6B21A8", "#C084FC"},
		{"success", dark.Success, light.Success, "#22C55E", "#15803D"},
		{"warning", dark.Warning, light.Warning, "#F59E0B", "#B45309"},
		{"error", dark.Error, light.Error, "#EF4444", "#DC2626"},
		{"info", dark.Info, light.Info, "#3B82F6", "#2563EB"},
		{"text", dark.Text, light.Text, "#F1F5F9", "#0F172A"},
		{"text_soft", dark.TextSoft, light.TextSoft, "#CBD5E1", "#334155"},
		{"text_dim", dark.TextDim, light.TextDim, "#94A3B8", "#475569"},
		{"text_faint", dark.TextFaint, light.TextFaint, "#64748B", "#64748B"},
		{"subtle", dark.Subtle, light.Subtle, "#475569", "#94A3B8"},
		{"rule", dark.Rule, light.Rule, "#334155", "#CBD5E1"},
		{"code", dark.Code, light.Code, "#22D3EE", "#0E7490"},
		{"accent", dark.Accent, light.Accent, "#06B6D4", "#0E7490"},
	} {
		if tc.dark != lipgloss.Color(tc.wantDark) {
			t.Errorf("dark %s = %v, want %s", tc.role, tc.dark, tc.wantDark)
		}
		if tc.light != lipgloss.Color(tc.wantLight) {
			t.Errorf("light %s = %v, want %s", tc.role, tc.light, tc.wantLight)
		}
	}
}

func TestResolveThemeANSI256UsesCuratedPins(t *testing.T) {
	th := ResolveTheme(colorprofile.ANSI256, true, ThemeDefault)
	if th.Primary != lipgloss.Color("135") {
		t.Errorf("ANSI256 dark Primary = %v, want index 135", th.Primary)
	}
	if th.Warning != lipgloss.Color("214") {
		t.Errorf("ANSI256 dark Warning = %v, want index 214", th.Warning)
	}
	if th.Text != lipgloss.Color("255") {
		t.Errorf("ANSI256 dark Text = %v, want index 255", th.Text)
	}

	lt := ResolveTheme(colorprofile.ANSI256, false, ThemeDefault)
	if lt.Text != lipgloss.Color("235") {
		t.Errorf("ANSI256 light Text = %v, want index 235", lt.Text)
	}
	if lt.Rule != lipgloss.Color("252") {
		t.Errorf("ANSI256 light Rule = %v, want index 252", lt.Rule)
	}
}

func TestResolveThemeANSI16InheritsTerminalPalette(t *testing.T) {
	th := ResolveTheme(colorprofile.ANSI, true, ThemeDefault)
	if th.Primary != lipgloss.Color("13") {
		t.Errorf("ANSI dark Primary = %v, want bright magenta (13)", th.Primary)
	}
	if th.Accent != lipgloss.Color("14") {
		t.Errorf("ANSI dark Accent = %v, want bright cyan (14)", th.Accent)
	}
	if _, ok := th.Text.(lipgloss.NoColor); !ok {
		t.Errorf("ANSI dark Text = %v, want NoColor (terminal default fg)", th.Text)
	}

	lt := ResolveTheme(colorprofile.ANSI, false, ThemeDefault)
	if lt.Primary != lipgloss.Color("5") {
		t.Errorf("ANSI light Primary = %v, want magenta (5)", lt.Primary)
	}
}

func TestResolveThemeHighContrastIsBackgroundIndependent(t *testing.T) {
	dark := ResolveTheme(colorprofile.TrueColor, true, ThemeHighContrast)
	light := ResolveTheme(colorprofile.TrueColor, false, ThemeHighContrast)
	if dark.Primary != light.Primary || dark.Primary != lipgloss.Color("#FF00FF") {
		t.Errorf("high-contrast Primary = %v / %v, want #FF00FF both", dark.Primary, light.Primary)
	}
	if dark.Text != lipgloss.Color("#FFFFFF") {
		t.Errorf("high-contrast Text = %v, want #FFFFFF", dark.Text)
	}
	if dark.Dark == light.Dark {
		t.Error("Dark field must still record the requested polarity")
	}
}

func TestUseThemeSwapsAtomically(t *testing.T) {
	t.Cleanup(func() { SetDarkBackground(true) })

	before := CurrentTheme()
	UseTheme(ResolveTheme(colorprofile.TrueColor, false, ThemeDefault))
	after := CurrentTheme()
	if before.Text == after.Text {
		t.Error("UseTheme did not swap the active theme")
	}
	if ColorText() != after.Text {
		t.Error("getter does not read the active theme")
	}
}
