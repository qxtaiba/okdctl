package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestErrorSummaryKindHeadlineAndHint(t *testing.T) {
	err := &errtypes.ConfigError{Msg: "ignition tls cert not found at /path/server.crt; re-run setup to regenerate it"}
	got := ErrorSummary(err, 2, "RUN123")
	for _, want := range []string{
		"ERROR",                         // box title
		"config error",                  // kind chip
		"ignition tls cert not found",   // headline
		"re-run setup to regenerate it", // promoted hint
		"exit 2",
		"RUN123",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error box missing %q:\n%s", want, got)
		}
	}
}

func TestErrorSummaryKindLabels(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&errtypes.ConfigError{Msg: "x"}, "config error"},
		{&errtypes.NetworkError{Msg: "x"}, "network error"},
		{&errtypes.ClusterError{Msg: "x"}, "cluster error"},
		{&errtypes.AuthError{Msg: "x"}, "auth error"},
		{&errtypes.UsageError{Msg: "x"}, "usage error"},
		{errors.New("plain boom"), "error"},
	}
	for _, c := range cases {
		got := ErrorSummary(c.err, 1, "R")
		if !strings.Contains(got, c.want) {
			t.Errorf("kind label for %T: want %q in:\n%s", c.err, c.want, got)
		}
	}
}

func TestErrorSummaryNoHintKeepsWholeMessage(t *testing.T) {
	got := ErrorSummary(errors.New("something broke with no semicolon"), 1, "R")
	if !strings.Contains(got, "something broke with no semicolon") {
		t.Errorf("headline should carry the whole message when no hint clause:\n%s", got)
	}
	if strings.Contains(got, "→") {
		t.Errorf("no next-step pointer expected when message has no hint:\n%s", got)
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
