package errtypes_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// TestErrorStringOmitsInner locks the contract that Error() does not
// interpolate the inner err: a credential-bearing inner error must not
// leak via .Error() string, only via the typed errors.Unwrap chain.
func TestErrorStringOmitsInner(t *testing.T) {
	inner := errors.New("user=admin password=hunter2")
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"config", &errtypes.ConfigError{Msg: "bad yaml", Err: inner}, "config error: bad yaml"},
		{"network", &errtypes.NetworkError{Msg: "dial failed", Err: inner}, "network error: dial failed"},
		{"cluster", &errtypes.ClusterError{Msg: "oc get failed", Err: inner}, "cluster error: oc get failed"},
		{"auth", &errtypes.AuthError{Msg: "sudo rejected", Err: inner}, "auth error: sudo rejected"},
		{"usage", &errtypes.UsageError{Msg: "unknown flag", Err: inner}, "usage error: unknown flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Error()
			if got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "hunter2") || strings.Contains(got, "password") {
				t.Fatalf("Error() leaked inner credential: %q", got)
			}
		})
	}
}

// TestUnwrapChainIntact verifies errors.Is still walks to the wrapped
// sentinel after the Error()-only change.
//
// The ctx-sentinel rows lock the signalExitCode invariant: every errtype
// must preserve context.Canceled and context.DeadlineExceeded identity
// through its Unwrap chain so SIGINT→130 / SIGTERM→143 mapping stays
// correct even when a cancellation is wrapped inside a typed error.
func TestUnwrapChainIntact(t *testing.T) {
	ctxCanceled := context.Canceled
	ctxDeadline := context.DeadlineExceeded

	type sentinelCase struct {
		name     string
		err      error
		sentinel error
		wantIs   bool
	}

	mkCases := func(errtype string, wrap func(sentinel error) error) []sentinelCase {
		return []sentinelCase{
			{errtype + "/canceled", wrap(ctxCanceled), ctxCanceled, true},
			{errtype + "/deadline", wrap(ctxDeadline), ctxDeadline, true},
			{errtype + "/unrelated", wrap(ctxCanceled), os.ErrNotExist, false},
		}
	}

	var cases []sentinelCase
	cases = append(cases, mkCases("ConfigError", func(s error) error {
		return &errtypes.ConfigError{Msg: "test", Err: s}
	})...)
	cases = append(cases, mkCases("NetworkError", func(s error) error {
		return &errtypes.NetworkError{Msg: "test", Err: s}
	})...)
	cases = append(cases, mkCases("ClusterError", func(s error) error {
		return &errtypes.ClusterError{Msg: "test", Err: s}
	})...)
	cases = append(cases, mkCases("AuthError", func(s error) error {
		return &errtypes.AuthError{Msg: "test", Err: s}
	})...)
	cases = append(cases, mkCases("UsageError", func(s error) error {
		return &errtypes.UsageError{Msg: "test", Err: s}
	})...)
	cases = append(cases, sentinelCase{
		name:     "ConfigError/os.ErrNotExist",
		err:      &errtypes.ConfigError{Msg: "open install-config.yaml", Err: os.ErrNotExist},
		sentinel: os.ErrNotExist,
		wantIs:   true,
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := errors.Is(tc.err, tc.sentinel)
			if got != tc.wantIs {
				t.Fatalf("errors.Is(%T, %v) = %v, want %v; Unwrap chain broken or unexpected match",
					tc.err, tc.sentinel, got, tc.wantIs)
			}
		})
	}
}

// TestUnwrapNilWhenNoInner ensures a type with no Err returns nil from
// Unwrap and plays nicely with errors.Is over a nil target.
func TestUnwrapNilWhenNoInner(t *testing.T) {
	e := &errtypes.ConfigError{Msg: "x"}
	if got := errors.Unwrap(e); got != nil {
		t.Fatalf("Unwrap() = %v, want nil", got)
	}
	if errors.Is(e, os.ErrNotExist) {
		t.Fatal("errors.Is matched a sentinel despite nil inner")
	}
}

// TestWithHintCarriesStructuredHint locks WithHint's contract: the hint rides
// in a separate field, never mutating Msg; the receiver is unchanged; the
// returned error keeps the same concrete type and Unwrap chain; and the hint
// surfaces only through Error() (as a "; <hint>" suffix) and Describe.
func TestWithHintCarriesStructuredHint(t *testing.T) {
	inner := errors.New("boom")
	cfg := &errtypes.ConfigError{Msg: "config broke", Err: inner}
	got := cfg.WithHint("try force-unlock")

	var gotCfg *errtypes.ConfigError
	if !errors.As(got, &gotCfg) {
		t.Fatalf("WithHint() = %v; want *errtypes.ConfigError", got)
	}
	if gotCfg.Msg != "config broke" {
		t.Errorf("Msg mutated by WithHint = %q; want unchanged %q", gotCfg.Msg, "config broke")
	}
	if got.Error() != "config error: config broke; try force-unlock" {
		t.Errorf("Error() = %q; want hint appended for the log surface", got.Error())
	}
	d, ok := errtypes.Describe(got)
	if !ok || d.Message != "config broke" || d.Hint != "try force-unlock" {
		t.Errorf("Describe() = %+v (ok=%v); want Message %q Hint %q",
			d, ok, "config broke", "try force-unlock")
	}
	if !errors.Is(got, inner) {
		t.Error("WithHint() broke the Unwrap chain to the inner error")
	}
	if cfg.Msg != "config broke" {
		t.Errorf("receiver mutated: Msg = %q; want unchanged %q", cfg.Msg, "config broke")
	}

	cluster := &errtypes.ClusterError{Msg: "cluster broke"}
	gotCluster := cluster.WithHint("try Y")
	var gotClusterErr *errtypes.ClusterError
	if !errors.As(gotCluster, &gotClusterErr) || gotClusterErr.Msg != "cluster broke" {
		t.Errorf("ClusterError.WithHint() mutated Msg = %v; want %q", gotCluster, "cluster broke")
	}
	if gotCluster.Error() != "cluster error: cluster broke; try Y" {
		t.Errorf("ClusterError.WithHint().Error() = %q; want hint suffix", gotCluster.Error())
	}
}

// TestWithHintUniformAcrossCategories locks that every category implements
// HintAppender and preserves its concrete type — the invariant WithLockHint's
// non-ConfigError callers depend on to keep their exit code.
func TestWithHintUniformAcrossCategories(t *testing.T) {
	cases := []struct {
		name string
		base errtypes.HintAppender
		want errtypes.Kind
	}{
		{"config", &errtypes.ConfigError{Msg: "m"}, errtypes.KindConfig},
		{"network", &errtypes.NetworkError{Msg: "m"}, errtypes.KindNetwork},
		{"cluster", &errtypes.ClusterError{Msg: "m"}, errtypes.KindCluster},
		{"auth", &errtypes.AuthError{Msg: "m"}, errtypes.KindAuth},
		{"usage", &errtypes.UsageError{Msg: "m"}, errtypes.KindUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.base.WithHint("do X")
			kind, ok := errtypes.Classify(got)
			if !ok || kind != tc.want {
				t.Fatalf("Classify(WithHint) = %v (ok=%v); want %v — type reclassified",
					kind, ok, tc.want)
			}
			d, _ := errtypes.Describe(got)
			if d.Hint != "do X" {
				t.Errorf("Describe().Hint = %q; want %q", d.Hint, "do X")
			}
		})
	}
}

func TestKindLabelAndExitCode(t *testing.T) {
	cases := []struct {
		kind     errtypes.Kind
		label    string
		exitCode int
	}{
		{errtypes.KindUnknown, "error", 1},
		{errtypes.KindConfig, "config error", 2},
		{errtypes.KindNetwork, "network error", 3},
		{errtypes.KindCluster, "cluster error", 4},
		{errtypes.KindAuth, "auth error", 5},
		{errtypes.KindUsage, "usage error", 64},
	}
	for _, tc := range cases {
		if got := tc.kind.Label(); got != tc.label {
			t.Errorf("Kind(%d).Label() = %q, want %q", tc.kind, got, tc.label)
		}
		if got := tc.kind.ExitCode(); got != tc.exitCode {
			t.Errorf("Kind(%d).ExitCode() = %d, want %d", tc.kind, got, tc.exitCode)
		}
	}
}

func TestClassifyAndDescribe(t *testing.T) {
	cases := []struct {
		name string
		err  error
		kind errtypes.Kind
		ok   bool
	}{
		{"config", &errtypes.ConfigError{Msg: "bad config"}, errtypes.KindConfig, true},
		{"network", &errtypes.NetworkError{Msg: "unreachable"}, errtypes.KindNetwork, true},
		{"cluster", &errtypes.ClusterError{Msg: "degraded"}, errtypes.KindCluster, true},
		{"auth", &errtypes.AuthError{Msg: "denied"}, errtypes.KindAuth, true},
		{"usage", &errtypes.UsageError{Msg: "bad flag"}, errtypes.KindUsage, true},
		{"unknown", errors.New("plain"), errtypes.KindUnknown, false},
		{"nil", nil, errtypes.KindUnknown, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, ok := errtypes.Classify(tc.err)
			if k != tc.kind || ok != tc.ok {
				t.Errorf("Classify() = (%d, %v), want (%d, %v)", k, ok, tc.kind, tc.ok)
			}
			d, dok := errtypes.Describe(tc.err)
			if dok != tc.ok || d.Kind != tc.kind {
				t.Errorf("Describe() = (%+v, %v), want kind %d ok %v", d, dok, tc.kind, tc.ok)
			}
		})
	}
}

func TestAuthErrorDescribeIncludesPath(t *testing.T) {
	d, ok := errtypes.Describe(&errtypes.AuthError{Msg: "refused", Path: "/etc/x"})
	if !ok || !strings.Contains(d.Message, "/etc/x") {
		t.Errorf("Describe(AuthError with Path) message = %q, want it to include the path", d.Message)
	}
}
