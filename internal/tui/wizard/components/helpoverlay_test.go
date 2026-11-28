package components

import (
	"fmt"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func overlayBindings() []KeyHint {
	return []KeyHint{
		{Key: "↑↓/tab", Help: "navigate"},
		{Key: "enter", Help: "continue"},
		{Key: "esc", Help: "back"},
		{Key: "ctrl+c", Help: "quit"},
		{Key: "?", Help: "help"},
	}
}

func TestRenderHelpOverlay_ListsEveryBinding(t *testing.T) {
	out := tuitest.StripANSI(RenderHelpOverlay(overlayBindings(), 80, 24))
	for _, b := range overlayBindings() {
		if !strings.Contains(out, b.Key) || !strings.Contains(out, b.Help) {
			t.Errorf("missing %q/%q in:\n%s", b.Key, b.Help, out)
		}
	}
}

func TestRenderHelpOverlay_GroupsScreenThenGlobal(t *testing.T) {
	out := tuitest.StripANSI(RenderHelpOverlay(overlayBindings(), 80, 24))

	screenIdx := strings.Index(out, "screen")
	globalIdx := strings.Index(out, "global")
	if screenIdx < 0 || globalIdx < 0 {
		t.Fatalf("missing section headers:\n%s", out)
	}
	if screenIdx >= globalIdx {
		t.Fatalf("expected \"screen\" before \"global\":\n%s", out)
	}

	screenSection := out[:globalIdx]
	if strings.Contains(screenSection, "quit") || strings.Contains(screenSection, "back") {
		t.Fatalf("global-only bindings leaked into the screen section:\n%s", out)
	}
}

func TestRenderHelpOverlay_SizedExactly(t *testing.T) {
	for _, sz := range [][2]int{{60, 20}, {80, 24}, {100, 30}, {120, 40}} {
		out := RenderHelpOverlay(overlayBindings(), sz[0], sz[1])
		tuitest.AssertFits(t, out, sz[0], sz[1])

		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != sz[1] {
			t.Errorf("%v: %d rows, want exactly %d", sz, len(lines), sz[1])
		}
	}
}

func TestRenderHelpOverlay_ListsAllBindingsEvenWhenRibbonTruncates(t *testing.T) {
	bindings := []KeyHint{
		{Key: "↑↓", Help: "navigate rows"},
		{Key: "tab", Help: "next section"},
		{Key: "space", Help: "toggle selection"},
		{Key: "a", Help: "add row"},
		{Key: "d", Help: "delete row"},
		{Key: "enter", Help: "continue"},
		{Key: "esc", Help: "back"},
		{Key: "ctrl+c", Help: "quit"},
		{Key: "?", Help: "help"},
	}

	const narrowWidth = 40

	overlay := tuitest.StripANSI(RenderHelpOverlay(bindings, narrowWidth, 24))
	for _, b := range bindings {
		if !strings.Contains(overlay, b.Key) || !strings.Contains(overlay, b.Help) {
			t.Errorf("overlay missing %q/%q at width %d:\n%s", b.Key, b.Help, narrowWidth, overlay)
		}
	}
	tuitest.AssertFits(t, RenderHelpOverlay(bindings, narrowWidth, 24), narrowWidth, 24)
}

func TestRenderHelpOverlay_TwoColumnsWhenOneWouldOverflowHeight(t *testing.T) {
	var many []KeyHint
	for i := 0; i < 12; i++ {
		many = append(many, KeyHint{Key: "k" + string(rune('a'+i)), Help: "does a thing " + string(rune('a'+i))})
	}
	many = append(many, KeyHint{Key: "esc", Help: "back"}, KeyHint{Key: "ctrl+c", Help: "quit"}, KeyHint{Key: "?", Help: "help"})

	const height = 16
	out := RenderHelpOverlay(many, 80, height)
	tuitest.AssertFits(t, out, 80, height)

	stripped := tuitest.StripANSI(out)
	for _, b := range many {
		if !strings.Contains(stripped, b.Key) {
			t.Errorf("two-column layout dropped %q:\n%s", b.Key, stripped)
		}
	}

	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != height {
		t.Fatalf("expected exactly %d rows, got %d", height, len(rows))
	}
	strippedRows := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	foundTwoPerRow := false
	for _, r := range strippedRows {
		hits := 0
		for _, b := range many[:12] {
			if strings.Contains(r, b.Key+" does a thing") {
				hits++
			}
		}
		if hits >= 2 {
			foundTwoPerRow = true
			break
		}
	}
	if !foundTwoPerRow {
		t.Fatalf("expected a row with two screen bindings side by side:\n%s", stripped)
	}
}

func TestRenderHelpOverlay_NoScreenSectionWhenAllGlobal(t *testing.T) {
	out := tuitest.StripANSI(RenderHelpOverlay([]KeyHint{
		{Key: "ctrl+c", Help: "quit"},
		{Key: "?", Help: "help"},
	}, 80, 24))

	if strings.Contains(out, "screen") {
		t.Fatalf("expected no \"screen\" header with only global bindings:\n%s", out)
	}
	if !strings.Contains(out, "global") {
		t.Fatalf("expected a \"global\" header:\n%s", out)
	}
}

func TestRenderHelpOverlay_ClampProtectsGlobalAndHintOverScreenRows(t *testing.T) {
	var screen []KeyHint
	for i := range 30 {
		screen = append(screen, KeyHint{Key: fmt.Sprintf("k%d", i), Help: "x"})
	}
	bindings := append(append([]KeyHint{}, screen...),
		KeyHint{Key: "esc", Help: "back"},
		KeyHint{Key: "ctrl+c", Help: "quit"},
		KeyHint{Key: "?", Help: "help"},
	)

	const width, height = 80, 18
	out := RenderHelpOverlay(bindings, width, height)
	tuitest.AssertFits(t, out, width, height)

	stripped := tuitest.StripANSI(out)
	if !strings.Contains(stripped, "global") {
		t.Fatalf("global section header must survive an overflowing screen section:\n%s", stripped)
	}
	if !strings.Contains(stripped, "ctrl+c") || !strings.Contains(stripped, "quit") {
		t.Fatalf("ctrl+c must survive an overflowing screen section:\n%s", stripped)
	}
	if !strings.Contains(stripped, "esc or ? closes") {
		t.Fatalf("the closing hint must survive an overflowing screen section:\n%s", stripped)
	}

	missing := 0
	for _, b := range screen {
		if !strings.Contains(stripped, b.Key+" x") {
			missing++
		}
	}
	if missing == 0 {
		t.Fatal("setup: expected the screen section to actually overflow and drop some items — strengthen the fixture")
	}
}
