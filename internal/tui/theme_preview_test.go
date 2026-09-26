package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestRenderThemePreviewHonorsOutputProfile(t *testing.T) {
	theme := ResolveTheme(colorprofile.TrueColor, true, ThemeDefault)
	colored := RenderThemePreview(&theme, colorprofile.TrueColor)
	if !strings.Contains(colored, "\x1b[") || !strings.Contains(colored, "OKDCTL cluster identity") {
		t.Fatalf("truecolor preview lacks styled TUI samples: %q", colored)
	}
	plain := RenderThemePreview(&theme, colorprofile.NoTTY)
	if strings.Contains(plain, "\x1b[") || !strings.Contains(plain, "OKDCTL cluster identity") {
		t.Fatalf("no-color preview = %q", plain)
	}

	ansiTheme := ResolveTheme(colorprofile.ANSI, true, ThemeDefault)
	ansi := RenderThemePreview(&ansiTheme, colorprofile.ANSI)
	if !strings.Contains(ansi, "\x1b[") || strings.Contains(ansi, "38;2;") {
		t.Fatalf("ANSI preview did not use terminal palette colors: %q", ansi)
	}
}
