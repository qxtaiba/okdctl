package cli

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestConfirmClusterMatches(t *testing.T) {
	cases := []struct {
		name    string
		force   bool
		confirm string
		cluster string
		verb    string
		wantErr bool
	}{
		{"force=false short-circuits", false, "", "prod", "cleanup", false},
		{"empty confirm with force", true, "", "prod", "cleanup", true},
		{"mismatched confirm", true, "staging", "prod", "cleanup", true},
		{"correct confirm", true, "prod", "prod", "cleanup", false},
		{"destroy verb", true, "prod", "prod", "destroy", false},
		{"destroy verb mismatch", true, "staging", "prod", "destroy", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := confirmClusterMatches(tc.force, tc.confirm, tc.cluster, tc.verb)
			if tc.wantErr {
				var cfgErr *errtypes.ConfigError
				if !errors.As(err, &cfgErr) {
					t.Fatalf("want *errtypes.ConfigError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil, got %v", err)
			}
		})
	}
}
