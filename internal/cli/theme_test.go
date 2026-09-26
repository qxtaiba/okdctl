package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui"
)

func TestThemePreviewGoldenAndReadOnly(t *testing.T) {
	original := tui.CurrentTheme()
	t.Cleanup(func() { tui.UseTheme(original) })
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })
	tui.UseTheme(tui.ResolveTheme(tui.OutputColorProfile(), true, tui.ThemeDefault))

	before := tui.CurrentTheme()
	generation := tui.ThemeGeneration()
	var out bytes.Buffer
	oldOut := themePreviewCmd.OutOrStdout()
	themePreviewCmd.SetOut(&out)
	t.Cleanup(func() { themePreviewCmd.SetOut(oldOut) })
	oldMode := themePreviewMode
	themePreviewMode = "all"
	t.Cleanup(func() { themePreviewMode = oldMode })

	if err := runThemePreview(themePreviewCmd, nil); err != nil {
		t.Fatalf("runThemePreview() error = %v", err)
	}
	want, err := os.ReadFile("testdata/theme_preview.golden")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("preview output mismatch (-got +want):\n%q\n%q", out.String(), string(want))
	}
	if tui.ThemeGeneration() != generation || tui.CurrentTheme() != before {
		t.Fatal("preview changed the active theme")
	}
}

func TestThemePreviewModeSelection(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })
	var out bytes.Buffer
	themePreviewCmd.SetOut(&out)
	t.Cleanup(func() { themePreviewCmd.SetOut(nil) })
	themePreviewMode = "light"
	t.Cleanup(func() { themePreviewMode = "all" })

	if err := runThemePreview(themePreviewCmd, nil); err != nil {
		t.Fatalf("runThemePreview() error = %v", err)
	}
	if got := out.String(); !bytes.Contains([]byte(got), []byte("\nlight\n")) || bytes.Contains([]byte(got), []byte("\ndark\n")) {
		t.Fatalf("--mode light output = %q", got)
	}
}

func TestThemePreviewRejectsUnknownMode(t *testing.T) {
	themePreviewMode = "auto"
	t.Cleanup(func() { themePreviewMode = "all" })
	if err := runThemePreview(themePreviewCmd, nil); err == nil {
		t.Fatal("runThemePreview() accepted an unknown mode")
	}
}
