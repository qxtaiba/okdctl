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
func TestUnwrapChainIntact(t *testing.T) {
	wrapped := &errtypes.ConfigError{Msg: "open install-config.yaml", Err: os.ErrNotExist}
	if !errors.Is(wrapped, os.ErrNotExist) {
		t.Fatal("errors.Is(wrapped, os.ErrNotExist) = false; Unwrap chain broken")
	}

	deadline := &errtypes.ClusterError{Msg: "bootstrap timed out", Err: context.DeadlineExceeded}
	if !errors.Is(deadline, context.DeadlineExceeded) {
		t.Fatal("errors.Is(deadline, context.DeadlineExceeded) = false; ctx sentinel no longer walks")
	}
	if errors.Is(deadline, os.ErrNotExist) {
		t.Fatal("errors.Is matched an unrelated sentinel")
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
