package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// fakeCLIAddon is a minimal addon.Addon used to populate the global registry
// for printAddonList tests; the cli package's test binary never imports
// addon/catalog, so the registry is otherwise empty.
type fakeCLIAddon struct{ meta addon.Metadata }

func (f *fakeCLIAddon) Info() addon.Metadata { return f.meta }

func (f *fakeCLIAddon) Install(context.Context, *addon.Environment) error {
	return nil
}

func (f *fakeCLIAddon) Verify(context.Context, *addon.Environment) error {
	return nil
}

func (f *fakeCLIAddon) Uninstall(context.Context, *addon.Environment) error {
	return nil
}

func registerFakeCLIAddon(t *testing.T, meta *addon.Metadata) {
	t.Helper()
	if addon.Get(meta.Name) != nil {
		return
	}
	if err := addon.Register(&fakeCLIAddon{meta: *meta}); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestPrintAddonListFootnote(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	registerFakeCLIAddon(t, &addon.Metadata{Name: "footnote-fake", DisplayName: "Footnote Fake"})

	cfg := &config.Config{}
	var buf bytes.Buffer
	if err := printAddonList(&buf, cfg); err != nil {
		t.Fatalf("printAddonList: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"NAME", "DISPLAY-NAME", "footnote-fake", "Footnote Fake", "IN-CONFIG reflects the configuration file only"} {
		if !strings.Contains(out, want) {
			t.Errorf("printAddonList output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("printAddonList must carry no ANSI under a no-color profile: %q", out)
	}
}

func TestAddonVerifyTableTruncatesError(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{})
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	longErr := strings.Repeat("x", 200)
	results := []addon.VerifyResult{
		{Name: "flux"},
		{Name: "secretstore", Err: errors.New(longErr)},
	}

	var buf bytes.Buffer
	failed, err := printAddonVerify(&buf, results)
	if err != nil {
		t.Fatalf("printAddonVerify: %v", err)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want header + 2 rows, got %d lines:\n%s", len(lines), buf.String())
	}
	okRow, failRow := lines[1], lines[2]

	if !strings.Contains(okRow, tui.IconSuccess) {
		t.Errorf("ok row missing IconSuccess: %q", okRow)
	}
	if !strings.Contains(failRow, tui.IconError) {
		t.Errorf("fail row missing IconError: %q", failRow)
	}
	if !strings.Contains(failRow, "…") {
		t.Errorf("fail row must be middle-truncated with an ellipsis: %q", failRow)
	}
	if strings.Contains(failRow, longErr) {
		t.Errorf("fail row must not carry the full untruncated error: %q", failRow)
	}
	// Bound derives from the table layout, not the 200-char error: widest NAME
	// cell (11, "secretstore") + the 2-space gap + the 60-col MaxColWidth cap.
	if w := lipgloss.Width(failRow); w > 11+2+60 {
		t.Errorf("fail row width %d exceeds the MaxColWidth-bounded row: %q", w, failRow)
	}
}

func TestPrintAddonVerifyEmpty(t *testing.T) {
	var buf bytes.Buffer
	failed, err := printAddonVerify(&buf, nil)
	if err != nil {
		t.Fatalf("printAddonVerify: %v", err)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}
	got := buf.String()
	if !strings.Contains(got, "no addons enabled") || !strings.Contains(got, "okdctl addon install --all") {
		t.Errorf("printAddonVerify(nil) = %q, want to contain %q and %q", got, "no addons enabled", "okdctl addon install --all")
	}
}
