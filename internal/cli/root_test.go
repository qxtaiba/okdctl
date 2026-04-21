package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// TestExitCodeForTaxonomy locks the published exit-code contract from
// root.go's package doc: ConfigError=2, NetworkError=3, ClusterError=4,
// AuthError=5, everything else=1. Scripts consuming okdctl's exit codes
// depend on this mapping; any change here is a user-facing break.
func TestExitCodeForTaxonomy(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"ConfigError", &errtypes.ConfigError{Msg: "bad yaml"}, 2},
		{"NetworkError", &errtypes.NetworkError{Msg: "dial refused"}, 3},
		{"ClusterError", &errtypes.ClusterError{Msg: "oc get failed"}, 4},
		{"AuthError", &errtypes.AuthError{Msg: "sudo rejected"}, 5},
		{"generic", errors.New("something else"), 1},
		{"wrappedConfig", fmt.Errorf("step: %w", &errtypes.ConfigError{Msg: "x"}), 2},
		{"wrappedAuth", fmt.Errorf("step: %w", &errtypes.AuthError{Msg: "x"}), 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCodeFor(tc.err); got != tc.want {
				t.Fatalf("exitCodeFor(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
