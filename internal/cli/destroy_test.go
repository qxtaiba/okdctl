package cli

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestValidateConfirmCluster(t *testing.T) {
	cases := []struct {
		name    string
		force   bool
		confirm string
		cluster string
		wantErr bool
	}{
		{"force=false short-circuits", false, "", "prod", false},
		{"empty confirm with force", true, "", "prod", true},
		{"mismatched confirm", true, "staging", "prod", true},
		{"correct confirm", true, "prod", "prod", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfirmCluster(tc.force, tc.confirm, tc.cluster)
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
