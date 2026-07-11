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
	// Preserve the original os.ErrNotExist positive case.
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

// TestWithHintAppendsMsgAndPreservesErr locks WithHint's copy semantics: the
// receiver is left unmodified, the returned error keeps the same concrete
// type and Unwrap chain, and hint is appended to Msg.
func TestWithHintAppendsMsgAndPreservesErr(t *testing.T) {
	inner := errors.New("boom")
	cfg := &errtypes.ConfigError{Msg: "config broke", Err: inner}
	got := cfg.WithHint("try force-unlock")

	var gotCfg *errtypes.ConfigError
	if !errors.As(got, &gotCfg) {
		t.Fatalf("WithHint() = %v; want *errtypes.ConfigError", got)
	}
	if gotCfg.Msg != "config broke; try force-unlock" {
		t.Errorf("Msg = %q; want %q", gotCfg.Msg, "config broke; try force-unlock")
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
	if !errors.As(gotCluster, &gotClusterErr) || gotClusterErr.Msg != "cluster broke; try Y" {
		t.Errorf("ClusterError.WithHint() = %v; want Msg %q", gotCluster, "cluster broke; try Y")
	}
}
