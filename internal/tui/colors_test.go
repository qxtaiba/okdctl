package tui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

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

func TestSetDarkBackgroundResolvesPolarity(t *testing.T) {
	t.Cleanup(func() { SetDarkBackground(true) })

	SetDarkBackground(false)
	if IsDarkBackground() {
		t.Error("IsDarkBackground() = true after SetDarkBackground(false)")
	}
	if ColorText() != lipgloss.Color("#0F172A") {
		t.Errorf("light ColorText() = %v, want #0F172A", ColorText())
	}
	if ColorTextDim() != lipgloss.Color("#475569") {
		t.Errorf("light ColorTextDim() = %v, want #475569", ColorTextDim())
	}
	if ColorTextFaint() != lipgloss.Color("#64748B") {
		t.Errorf("light ColorTextFaint() = %v, want #64748B", ColorTextFaint())
	}
	if ColorRule() != lipgloss.Color("#CBD5E1") {
		t.Errorf("light ColorRule() = %v, want #CBD5E1", ColorRule())
	}

	SetDarkBackground(true)
	if !IsDarkBackground() {
		t.Error("IsDarkBackground() = false after SetDarkBackground(true)")
	}
	if ColorText() != lipgloss.Color("#F1F5F9") {
		t.Errorf("dark ColorText() = %v, want #F1F5F9", ColorText())
	}
	if ColorTextDim() != lipgloss.Color("#94A3B8") {
		t.Errorf("dark ColorTextDim() = %v, want #94A3B8", ColorTextDim())
	}
	if ColorTextFaint() != lipgloss.Color("#64748B") {
		t.Errorf("dark ColorTextFaint() = %v, want #64748B", ColorTextFaint())
	}
	if ColorRule() != lipgloss.Color("#334155") {
		t.Errorf("dark ColorRule() = %v, want #334155", ColorRule())
	}
}

func TestThemeRemapReachesRenderedRows(t *testing.T) {
	forced := colorprofile.TrueColor
	outputProfile.Store(&forced)
	t.Cleanup(func() {
		SetColorProfileFor(&bytes.Buffer{})
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
	sub := DefaultFactStyles().SubKey.Render("password")
	if !strings.Contains(sub, "100;116;139") {
		t.Fatalf("light nested key does not carry the Slate500 faint tier: %q", sub)
	}

	UseTheme(ResolveTheme(colorprofile.TrueColor, true, ThemeHighContrast))
	if code := CodeInlineStyle.Render("kubeadmin-password"); strings.Contains(code, "34;211;238") {
		t.Fatalf("high-contrast CodeInline still renders Cyan400: %q", code)
	}
	row = DottedKeyValueFull("console", "https://example", 12, 60)
	if strings.Contains(row, "148;163;184") || strings.Contains(row, "71;85;105") {
		t.Fatalf("high-contrast dottedKV key still renders a slate tier: %q", row)
	}
	sub = DefaultFactStyles().SubKey.Render("password")
	if strings.Contains(sub, "100;116;139") {
		t.Fatalf("high-contrast nested key still renders the slate faint tier: %q", sub)
	}
}

func TestHighContrastPropagatesToBaseStyles(t *testing.T) {
	t.Cleanup(func() { SetDarkBackground(true) })

	UseTheme(ResolveTheme(colorprofile.TrueColor, true, ThemeHighContrast))

	if got := TitleStyle.GetForeground(); got != lipgloss.Color("#FF00FF") {
		t.Errorf("TitleStyle.GetForeground() = %v, want #FF00FF", got)
	}
}
