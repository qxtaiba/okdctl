package render

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
)

func TestErrorSummaryKindHeadlineAndHint(t *testing.T) {
	err := (&errtypes.ConfigError{Msg: "ignition tls cert not found at /path/server.crt"}).
		WithHint("re-run setup to regenerate it")
	got := ErrorSummary(err, 2, "RUN123")
	for _, want := range []string{
		"ERROR",
		"config error",
		"ignition tls cert not found",
		"re-run setup to regenerate it",
		"→",
		"exit 2",
		"RUN123",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error box missing %q:\n%s", want, got)
		}
	}
}

// Locks that "; " in a message with no structured hint stays whole, not fabricated into one.
func TestErrorSummaryHintlessKeepsWholeMessage(t *testing.T) {
	const semicolonMsg = "node drained (2 pods evicted; 1 pending) before removal"
	cases := []struct {
		name string
		err  error
		exit int
		msg  string
	}{
		{"in-message semicolon is not a hint", &errtypes.ClusterError{Msg: semicolonMsg}, 4, semicolonMsg},
		{"plain message with no hint clause", errors.New("something broke with no semicolon"), 1, "something broke with no semicolon"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ErrorSummary(tc.err, tc.exit, "R")
			if !strings.Contains(got, tc.msg) {
				t.Errorf("headline should carry the whole message %q:\n%s", tc.msg, got)
			}
			if strings.Contains(got, "→") {
				t.Errorf("no next-step pointer expected; message carries no structured hint:\n%s", got)
			}
		})
	}
}

func TestPresentedMarkerRoundTrips(t *testing.T) {
	base := &errtypes.ClusterError{Msg: "boom"}
	wrapped := Presented(base)
	if !IsPresented(wrapped) {
		t.Fatal("IsPresented should report true for a Presented error")
	}
	var ce *errtypes.ClusterError
	if !errors.As(wrapped, &ce) {
		t.Fatal("errors.As must still reach the underlying errtypes value through Presented")
	}
	if Presented(nil) != nil {
		t.Fatal("Presented(nil) must be nil")
	}
}

func TestErrorCardOmitsRunID(t *testing.T) {
	got := ErrorCard("resize failed", "etcd health gate (post-master0) failed: quorum lost",
		"re-run the same operation to resume at the recorded step", 70)
	for _, want := range []string{"resize failed", "quorum lost", "→", "re-run the same operation"} {
		if !strings.Contains(got, want) {
			t.Errorf("error card missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{"run_id", "exit "} {
		if strings.Contains(got, absent) {
			t.Errorf("error card must carry no exit/run_id footer, found %q:\n%s", absent, got)
		}
	}
}

func TestErrorBodyChipHasTwoSpaceGap(t *testing.T) {
	summary := ErrorSummary(&errtypes.ConfigError{Msg: "boom"}, 1, "R")
	if !strings.Contains(summary, "✗  config error") {
		t.Errorf("ErrorSummary chip must render the icon and kind with a two-space gap:\n%s", summary)
	}
	card := ErrorCard("resize failed", "boom", "", 70)
	if !strings.Contains(card, "✗  resize failed") {
		t.Errorf("ErrorCard chip must render the icon and kind with a two-space gap:\n%s", card)
	}
}

// TestErrorCardWrappedHintNeverTouchesTheRightBorder guards the T8 nit. A
// single unbroken "word" longer than the wrap width hard-splits (see
// TestWrapTextHardSplitsLongToken) into lines that fill the wrap budget
// exactly — the worst case for a right-edge gutter — so every wrapped hint
// row this produces must still leave at least one blank column before the
// closing "│", never land flush against it the way message/kind rows can.
func TestErrorCardWrappedHintNeverTouchesTheRightBorder(t *testing.T) {
	hint := strings.Repeat("a", 200)
	out := ErrorCard("usage error", "short message", hint, 70)
	stripped := tuitest.StripANSI(out)

	found := false
	for _, line := range strings.Split(stripped, "\n") {
		if !strings.Contains(line, "a") || !strings.HasPrefix(line, "│") {
			continue
		}
		interior := strings.TrimSuffix(strings.TrimPrefix(line, "│"), "│")
		if !strings.Contains(interior, "aaa") {
			continue
		}
		found = true
		if !strings.HasSuffix(interior, " ") {
			t.Errorf("hint row has no gutter before the border: %q", line)
		}
	}
	if !found {
		t.Fatal("could not locate the wrapped hint's rows in the rendered card")
	}
}

func TestErrorSummaryGoldenAtWidths(t *testing.T) {
	err := (&errtypes.ConfigError{Msg: "ignition tls cert not found at /path/server.crt"}).
		WithHint("re-run setup to regenerate it")

	for _, w := range []int{60, 120} {
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			tui.SetTerminalWidth(w)
			t.Cleanup(func() { tui.SetTerminalWidth(0) })

			out := ErrorSummary(err, 2, "RUN123")
			tuitest.AssertFits(t, out, w, 0)
			tuitest.Golden(t, fmt.Sprintf("error_summary_%d", w), out)
		})
	}
}

// TestErrorSummarySanitizesHostileBackendText proves a backend error whose
// Error() text carries an OSC 0 sequence (sets the window title) never
// reaches the rendered error box as a live escape: describeError's fallback
// for an untyped error is the one generic sink every CLI command failure
// flows through (ErrorSummary -> describeError), so this exercises it
// directly rather than one of its many callers.
func TestErrorSummarySanitizesHostileBackendText(t *testing.T) {
	const payload = "\x1b]0;pwned\x07"
	err := errors.New("connection refused" + payload + " while contacting proxmox")

	out := ErrorSummary(err, 1, "run-1")

	if strings.Contains(out, payload) {
		t.Fatalf("rendered error box carries the raw osc payload:\n%q", out)
	}
	if strings.Contains(out, "pwned") {
		t.Fatalf("osc payload text leaked into the rendered error box:\n%q", out)
	}
	if !strings.Contains(out, "�") {
		t.Fatalf("rendered error box shows no sanitization marker:\n%q", out)
	}
	if !strings.Contains(out, "connection refused") || !strings.Contains(out, "while contacting proxmox") {
		t.Fatalf("rendered error box lost the legitimate message text:\n%q", out)
	}
}

func TestWrapTextHardSplitsLongToken(t *testing.T) {
	long := strings.Repeat("a", 50)
	lines := tui.WrapLines(long, 20)
	for _, l := range lines {
		if w := lipgloss.Width(l); w > 20 {
			t.Fatalf("WrapLines produced a %d-col line over the 20 budget: %q", w, l)
		}
	}
	if joined := strings.Join(lines, ""); joined != long {
		t.Fatalf("WrapLines lost characters: %q", joined)
	}
}
