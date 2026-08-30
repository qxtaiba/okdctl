package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
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

func TestWrapTextHardSplitsLongToken(t *testing.T) {
	long := strings.Repeat("a", 50)
	lines := wrapText(long, 20)
	for _, l := range lines {
		if len(l) > 20 {
			t.Fatalf("wrapText produced a %d-col line over the 20 budget: %q", len(l), l)
		}
	}
	if joined := strings.Join(lines, ""); joined != long {
		t.Fatalf("wrapText lost characters: %q", joined)
	}
}
