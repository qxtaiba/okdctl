package cli

import (
	"errors"
	"runtime"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

func TestDoctorExitErr_ExitCodeMapping(t *testing.T) {
	cases := []struct {
		name  string
		fails int
		warns int
		want  int
	}{
		{"clean", 0, 0, 0},
		{"warn only", 0, 1, 6},
		{"fail beats warn", 1, 1, 2},
		{"fail only", 2, 0, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCodeFor(doctorExitErr(tc.fails, tc.warns)); got != tc.want {
				t.Errorf("exit code = %d; want %d", got, tc.want)
			}
		})
	}
}

// TestRunDoctorNonLinuxGateIsUsageError is meaningful only off linux, where the
// OS gate is reachable.
func TestRunDoctorNonLinuxGateIsUsageError(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("gate is unreachable on linux")
	}
	err := runDoctor(doctorCmd, nil)
	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("want *errtypes.UsageError (exit 64), got %T: %v", err, err)
	}
}
