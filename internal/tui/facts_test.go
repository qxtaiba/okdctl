package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestRenderFactsDotsLayout(t *testing.T) {
	lines := RenderFacts(
		[]FactRow{{Key: "console", Value: "https://x"}},
		&FactLayout{Leader: FactLeaderDots, KeyWidth: 12, Styles: DefaultFactStyles()},
	)
	if len(lines) != 1 {
		t.Fatalf("lines = %q, want one unwrapped row", lines)
	}
	if got := tuitest.StripANSI(lines[0]); got != "console ... https://x" {
		t.Errorf("dots row = %q, want the dotted-leader layout", got)
	}
}

func TestRenderFactsDotsWrapsUnderValueColumn(t *testing.T) {
	lines := RenderFacts(
		[]FactRow{{Key: "path", Value: "alpha beta gamma delta"}},
		&FactLayout{Leader: FactLeaderDots, KeyWidth: 10, TotalWidth: 24, Styles: DefaultFactStyles()},
	)
	if len(lines) < 2 {
		t.Fatalf("lines = %q, want the value wrapped across rows", lines)
	}
	if got := tuitest.StripANSI(lines[1]); !strings.HasPrefix(got, strings.Repeat(" ", 10)) {
		t.Errorf("continuation = %q, want it indented under the value column", got)
	}
}

func TestRenderFactsPadLayout(t *testing.T) {
	lines := RenderFacts(
		[]FactRow{{Key: "name", Value: "homelab"}},
		&FactLayout{Leader: FactLeaderPad, KeyWidth: 10, Styles: DefaultFactStyles()},
	)
	if got := tuitest.StripANSI(lines[0]); got != "name      homelab" {
		t.Errorf("pad row = %q, want the key padded to its column", got)
	}
}

func TestRenderFactsColonFlowWrapsWithStyledSeam(t *testing.T) {
	lines := RenderFacts(
		[]FactRow{{Key: "domain", Value: "lab.example.com"}},
		&FactLayout{Leader: FactLeaderColon, TotalWidth: 12, Styles: DefaultFactStyles()},
	)
	if len(lines) < 2 {
		t.Fatalf("lines = %q, want the flow-wrapped rows", lines)
	}
	if got := tuitest.StripANSI(lines[0]); got != "domain:" {
		t.Errorf("first row = %q, want the flow wrap to break after the key", got)
	}
	if got := tuitest.StripANSI(lines[1]); !strings.HasPrefix(got, "lab.") {
		t.Errorf("second row = %q, want the value flowing on", got)
	}
}

func TestRenderFactsSubAndHighlightSelectStyles(t *testing.T) {
	styles := DefaultFactStyles()
	rows := RenderFacts(
		[]FactRow{
			{Key: "plain", Value: "v"},
			{Key: "nested", Value: "v", Sub: true},
			{Key: "hot", Value: "v", Highlight: true},
		},
		&FactLayout{Leader: FactLeaderDots, KeyWidth: 10, Styles: styles},
	)
	if !strings.Contains(rows[1], styles.SubKey.Render("nested")) {
		t.Errorf("sub row = %q, want the SubKey style on the key", rows[1])
	}
	if !strings.Contains(rows[2], styles.Highlight.Render("v")) {
		t.Errorf("highlight row = %q, want the Highlight style on the value", rows[2])
	}
}

func TestHyperlinkGatesOnColorProfile(t *testing.T) {
	forced := colorprofile.TrueColor
	outputProfile.Store(&forced)
	t.Cleanup(func() { SetColorProfileFor(&bytes.Buffer{}) })

	linked := Hyperlink("https://example.com", "console")
	if !strings.Contains(linked, "\x1b]8;;https://example.com\x1b\\") || !strings.Contains(linked, "console") {
		t.Fatalf("Hyperlink = %q, want the OSC 8 wrap around the text", linked)
	}

	DisableColor()
	if got := Hyperlink("https://example.com", "console"); got != "console" {
		t.Fatalf("Hyperlink under NO_COLOR/off-TTY = %q, want plain text", got)
	}
}

func TestRenderFactsLinksEveryValueLine(t *testing.T) {
	forced := colorprofile.TrueColor
	outputProfile.Store(&forced)
	t.Cleanup(func() { SetColorProfileFor(&bytes.Buffer{}) })

	url := "https://console-openshift-console.apps.lab.example.com"
	lines := RenderFacts(
		[]FactRow{{Key: "console", Value: url, Link: url}},
		&FactLayout{Leader: FactLeaderDots, KeyWidth: 12, TotalWidth: 40, Styles: DefaultFactStyles()},
	)
	if len(lines) < 2 {
		t.Fatalf("lines = %q, want the URL wrapped across rows", lines)
	}
	for i, l := range lines {
		if !strings.Contains(l, "\x1b]8;;"+url+"\x1b\\") {
			t.Errorf("line %d = %q, want each wrapped segment linked to the full URL", i, l)
		}
	}
}
