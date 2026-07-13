package cli

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// TestDoctorExitErr locks the tri-state contract shared by the JSON and text
// rendering paths: clean run, warn-only run, and any-fail run must each map
// to a distinct sentinel so exitCodeFor can resolve 0/6/2.
func TestDoctorExitErr(t *testing.T) {
	t.Run("all pass returns nil", func(t *testing.T) {
		if err := doctorExitErr(0, 0); err != nil {
			t.Fatalf("doctorExitErr(0, 0) = %v; want nil", err)
		}
	})

	t.Run("warn only returns errDoctorWarn", func(t *testing.T) {
		err := doctorExitErr(0, 2)
		if !errors.Is(err, errDoctorWarn) {
			t.Fatalf("doctorExitErr(0, 2) = %v; want errDoctorWarn", err)
		}
	})

	t.Run("any fail returns ConfigError regardless of warns", func(t *testing.T) {
		for _, warns := range []int{0, 3} {
			err := doctorExitErr(1, warns)
			var cfgErr *errtypes.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("doctorExitErr(1, %d) = %v (%T); want *errtypes.ConfigError", warns, err, err)
			}
		}
	})
}

// TestDoctorExitErr_ExitCodeMapping verifies the full path from tallies to
// process exit code: 0 = clean, 6 = warn-only, 2 = any fail.
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
