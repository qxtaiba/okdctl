package wizard

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

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
	// ribbonItems' one essential entry ("?") is reserved, so truncation eats
	// into the rest and the ellipsis now sits before it, not at the very end.
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

// TestRenderHelpRibbon_EssentialsNeverDropped pins the ESSENTIALS-LAST-DROPPED
// ordering rule: ctrl+c sits mid-list (not last), followed by several
// non-essential items — under the old left-to-right greedy-drop, overflowing
// on ctrl+c's own slot would have dropped it and everything after. At this
// narrow width the ribbon must still truncate (something has to give) but
// both essential items — ctrl+c and the trailing "?" — must survive.
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
		// The earliest non-essential item should still have room — this pins
		// that truncation eats the middle of rest, not the front of it.
		t.Fatalf("expected the first non-essential item to survive: %q", out)
	}
	if strings.Contains(out, "retry") || strings.Contains(out, "add row") || strings.Contains(out, "delete row") {
		t.Fatalf("expected later non-essential items to be the ones dropped: %q", out)
	}
}

// TestRenderHelpRibbon_BothEssentialsSurviveAtFloorWidth pins the extreme
// case: a width so narrow only the two essential items themselves fit —
// every non-essential item is dropped, but ctrl+c and ? never are.
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

// essentialOnlyItems is ribbonItems' essential pair alone — "ctrl+c quit •
// ? help" needs 20 columns fully spelled out, so any width below that
// forces RenderHelpRibbon's negative-budget degradation path even before
// touching a single non-essential item.
func essentialOnlyItems() []KeyBinding {
	return []KeyBinding{
		{Key: HelpCtrlC, Help: HelpQuit},
		{Key: HelpQuestion, Help: HelpOverlay},
	}
}

// TestRenderHelpRibbon_NegativeBudgetNeverBareClips pins the fix for the
// preview-blockers bug: a step's own wide PinnedFooter can leave the ribbon
// only a handful of columns — fewer than even the essential pair alone
// needs. RenderHelpRibbon must never hand back a bare mid-word clip (e.g.
// "ctr") in that case; below the floor it's a single honest "…", above it
// it's as much of the essentials as fits with a trailing "…".
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
		// The bug this pins: a bare MaxWidth clip lands mid-word with no
		// ellipsis at all — "ctr" out of "ctrl+c quit • ? help" being the
		// reported case. Any survivor that doesn't end in "…" is exactly
		// that failure mode.
		if strings.HasPrefix(out, "ctr") && !strings.Contains(out, "…") {
			t.Errorf("width=%d: bare mid-word clip, not an honest truncation: %q", width, out)
		}
	}
}

// TestRenderHelpRibbon_NegativeBudgetFloorIsJustEllipsis pins the sub-floor
// case explicitly: below ~6 columns there's no room for any of the
// essentials' own text, so RenderHelpRibbon falls back to the single "…"
// marker rather than a truncated fragment of "ctrl+c" or "?".
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

// TestRebuildWizardStylesRebindsInitCapturedStyles pins the same flip for
// the wizard chrome and the data-driven form's style cache: brand, accent,
// and status roles all rebind on light, so nothing may freeze init values.
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
