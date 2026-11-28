package wizard

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func ribbonItems() []KeyBinding {
	return []KeyBinding{
		{"↑/↓", "navigate"},
		{"tab", "next"},
		{"enter", "continue"},
		{"esc", "back"},
		{"?", "help"},
		{"pgup/pgdn", "scroll"},
	}
}

func TestRenderHelpRibbon_FitsExactly(t *testing.T) {
	out := tuitest.StripANSI(RenderHelpRibbon(ribbonItems(), 40))
	if lipgloss.Width(out) > 40 || strings.Contains(out, "\n") || !strings.Contains(out, "…") {
		t.Fatalf("%q", out)
	}
	if !strings.HasSuffix(out, "? help") {
		t.Fatalf("essential item must trail, even truncated: %q", out)
	}
}

func TestRenderHelpRibbon_NoEllipsisWhenAllFit(t *testing.T) {
	items := ribbonItems()
	out := tuitest.StripANSI(RenderHelpRibbon(items, 120))
	if strings.Contains(out, "…") {
		t.Fatalf("unexpected ellipsis: %q", out)
	}
	for _, it := range items {
		if !strings.Contains(out, it.Key) || !strings.Contains(out, it.Help) {
			t.Fatalf("missing %q/%q in %q", it.Key, it.Help, out)
		}
	}
}

func TestRenderHelpRibbon_EssentialsNeverDropped(t *testing.T) {
	items := []KeyBinding{
		{Key: "↑↓", Help: HelpNavigate},
		{Key: "tab", Help: "next section"},
		{Key: HelpCtrlC, Help: HelpQuit},
		{Key: "r", Help: "retry"},
		{Key: "a", Help: "add row"},
		{Key: "d", Help: "delete row"},
		{Key: HelpQuestion, Help: HelpOverlay},
	}

	out := tuitest.StripANSI(RenderHelpRibbon(items, 36))
	if lipgloss.Width(out) > 36 {
		t.Fatalf("row too wide: %q", out)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("expected truncation at width 36: %q", out)
	}
	if !strings.Contains(out, HelpCtrlC) {
		t.Fatalf("ctrl+c dropped despite being essential: %q", out)
	}
	if !strings.HasSuffix(out, HelpQuestion+" "+HelpOverlay) {
		t.Fatalf("essential items must trail the ribbon, ctrl+c then ? help: %q", out)
	}
	if !strings.Contains(out, "navigate") {
		t.Fatalf("expected the first non-essential item to survive: %q", out)
	}
	if strings.Contains(out, "retry") || strings.Contains(out, "add row") || strings.Contains(out, "delete row") {
		t.Fatalf("expected later non-essential items to be the ones dropped: %q", out)
	}
}

func TestRenderHelpRibbon_BothEssentialsSurviveAtFloorWidth(t *testing.T) {
	items := []KeyBinding{
		{Key: "↑↓", Help: HelpNavigate},
		{Key: HelpEnter, Help: HelpConfirm},
		{Key: HelpEsc, Help: HelpBack},
		{Key: HelpCtrlC, Help: HelpQuit},
		{Key: HelpQuestion, Help: HelpOverlay},
	}

	out := tuitest.StripANSI(RenderHelpRibbon(items, 22))
	if lipgloss.Width(out) > 22 {
		t.Fatalf("row too wide: %q", out)
	}
	if !strings.Contains(out, HelpCtrlC) || !strings.Contains(out, HelpQuestion+" "+HelpOverlay) {
		t.Fatalf("both essential items must survive: %q", out)
	}
}

func essentialOnlyItems() []KeyBinding {
	return []KeyBinding{
		{Key: HelpCtrlC, Help: HelpQuit},
		{Key: HelpQuestion, Help: HelpOverlay},
	}
}

func TestRenderHelpRibbon_NegativeBudgetNeverBareClips(t *testing.T) {
	items := essentialOnlyItems()

	for _, width := range []int{3, 8, 12} {
		out := tuitest.StripANSI(RenderHelpRibbon(items, width))

		if lipgloss.Width(out) > width {
			t.Errorf("width=%d: row too wide: %q", width, out)
		}
		if out == "" {
			t.Errorf("width=%d: got empty output", width)
			continue
		}
		if !strings.HasSuffix(out, "…") {
			t.Errorf("width=%d: degraded output must end in an honest ellipsis, got %q", width, out)
		}
		if strings.HasPrefix(out, "ctr") && !strings.Contains(out, "…") {
			t.Errorf("width=%d: bare mid-word clip, not an honest truncation: %q", width, out)
		}
	}
}

func TestRenderHelpRibbon_NegativeBudgetFloorIsJustEllipsis(t *testing.T) {
	items := essentialOnlyItems()

	for _, width := range []int{1, 2, 3, 4, 5} {
		out := tuitest.StripANSI(RenderHelpRibbon(items, width))
		if lipgloss.Width(out) > width {
			t.Errorf("width=%d: row too wide: %q", width, out)
		}
		if out != "…" {
			t.Errorf("width=%d: want just the ellipsis marker, got %q", width, out)
		}
	}
}

func TestRebuildWizardStylesRebindsInitCapturedStyles(t *testing.T) {
	t.Cleanup(resetPackageColorState)

	tui.SetDarkBackground(false)
	rebuildWizardStyles()

	if logo := LogoStyle.Render("OKDCTL"); !strings.Contains(logo, "126;34;206") {
		t.Errorf("logo = %q, want the light Primary tier (#7E22CE)", logo)
	}
	if tag := TaglineStyle.Render("okd over proxmox"); !strings.Contains(tag, "71;85;105") {
		t.Errorf("tagline = %q, want the light TextDim tier (#475569)", tag)
	}
	if head := formViewStyles.sectionHeader.Render("connection"); !strings.Contains(head, "14;116;144") {
		t.Errorf("form section header = %q, want the light Accent tier (#0E7490)", head)
	}
	if dot := StepDotPendingStyle.Render("o"); !strings.Contains(dot, "148;163;184") {
		t.Errorf("pending dot = %q, want the light Subtle tier (#94A3B8)", dot)
	}
	if label := stageLabelCurrentStyle.Render("connect"); !strings.Contains(label, "15;23;42") {
		t.Errorf("current stage label = %q, want the light Text tier (#0F172A)", label)
	}
	if sep := stageSeparatorStyle.Render(" · "); !strings.Contains(sep, "148;163;184") {
		t.Errorf("stage separator = %q, want the light Subtle tier (#94A3B8)", sep)
	}
}

func TestWizardChromeBrandRampFollowsThemePolarity(t *testing.T) {
	t.Cleanup(resetPackageColorState)
	for _, tc := range []struct {
		name string
		dark bool
		ansi string
	}{
		{name: "dark", dark: true, ansi: "38;2;107;33;168"},
		{name: "light", ansi: "38;2;192;132;252"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tui.UseTheme(tui.ResolveTheme(colorprofile.TrueColor, tc.dark, tui.ThemeDefault))
			rebuildWizardStyles()
			frame := WizardBorderStyle.Width(12).Render("frame")
			header := HeaderStyle.Width(12).Render("header")
			if !strings.Contains(frame, tc.ansi) {
				t.Errorf("frame border = %q, want PrimaryDim %s", frame, tc.ansi)
			}
			if !strings.Contains(header, tc.ansi) {
				t.Errorf("header rule = %q, want PrimaryDim %s", header, tc.ansi)
			}
		})
	}
}

func TestWizardChromeHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Cleanup(func() {
		tui.SetColorProfileFor(&bytes.Buffer{})
		resetPackageColorState()
	})
	t.Setenv("NO_COLOR", "1")
	tui.SetColorProfileFor(io.Discard)
	rebuildWizardStyles()

	frame := WizardBorderStyle.Width(12).Render("frame") + HeaderStyle.Width(12).Render("header")
	frame = tui.Downsample(frame)
	if strings.Contains(frame, "\x1b[") {
		t.Fatalf("wizard chrome emitted ANSI with NO_COLOR: %q", frame)
	}
}
