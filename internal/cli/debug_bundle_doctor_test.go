//go:build linux

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain implements the subprocess-hijack used to fake the re-exec inside
// collectDoctorOutput. When TEST_DOCTOR_SUBPROCESS is set, this process IS
// the fake doctor command: it writes the env value to stdout and exits 1 to
// simulate a failing preflight. Normal test invocations skip this branch.
func TestMain(m *testing.M) {
	if val, ok := os.LookupEnv("TEST_DOCTOR_SUBPROCESS"); ok {
		fmt.Print(val)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestCollectDoctorOutputBuffersOnFail(t *testing.T) {
	const want = "doctor: preflight failed\n"
	t.Setenv("TEST_DOCTOR_SUBPROCESS", want)
	buf, err := collectDoctorOutput(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(buf), want) {
		t.Errorf("buffer = %q, want it to contain %q", buf, want)
	}
}

func TestCollectDoctorOutputEmptyIsNonNil(t *testing.T) {
	t.Setenv("TEST_DOCTOR_SUBPROCESS", "")
	buf, err := collectDoctorOutput(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf == nil {
		t.Error("non-nil buffer expected even when subprocess emits no output")
	}
}
