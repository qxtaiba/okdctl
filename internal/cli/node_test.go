package cli

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestValidateResizeFlags(t *testing.T) {
	cases := []struct {
		name          string
		memoryMB, cpu int
		wantErr       bool
	}{
		{"memory only", 16384, 0, false},
		{"cpu only", 0, 8, false},
		{"both set", 16384, 8, false},
		{"neither set", 0, 0, true},
		{"negative values treated as unset", -1, -1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateResizeFlags(tc.memoryMB, tc.cpu)
			if tc.wantErr {
				var usageErr *errtypes.UsageError
				if !errors.As(err, &usageErr) {
					t.Fatalf("want *errtypes.UsageError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
		})
	}
}

func TestDescribeResizeChange(t *testing.T) {
	cases := []struct {
		name          string
		memoryMB, cpu int
		want          string
	}{
		{"memory only", 16384, 0, "16384 MiB"},
		{"cpu only", 0, 8, "8 vCPU"},
		{"both", 16384, 8, "16384 MiB, 8 vCPU"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeResizeChange(tc.memoryMB, tc.cpu); got != tc.want {
				t.Errorf("describeResizeChange(%d, %d) = %q, want %q", tc.memoryMB, tc.cpu, got, tc.want)
			}
		})
	}
}
