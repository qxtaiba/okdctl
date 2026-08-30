package debugbundle

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain re-execs as a fake `doctor` command (argv[1] == "doctor"): writes
// markers to stdout/stderr plus a canary env var, exits 1.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		fmt.Print("fake-doctor-stdout")
		if v := os.Getenv("TEST_DOCTOR_CANARY"); v != "" {
			fmt.Print(v)
		}
		fmt.Fprint(os.Stderr, "fake-doctor-stderr")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestCollectDoctorOutputSeparatesStreamsAndToleratesFailure(t *testing.T) {
	stdout, stderr, err := collectDoctorOutput(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "fake-doctor-stdout" {
		t.Errorf("stdout = %q, want %q with no stderr interleaved", stdout, "fake-doctor-stdout")
	}
	// Prefix, not equality: `go test -cover` appends a GOCOVERDIR warning after the marker.
	if !strings.HasPrefix(string(stderr), "fake-doctor-stderr") {
		t.Errorf("stderr = %q, want %q prefix", stderr, "fake-doctor-stderr")
	}
	if strings.Contains(string(stderr), "fake-doctor-stdout") {
		t.Errorf("stdout marker leaked into stderr: %q", stderr)
	}
}

func TestCollectDoctorOutputFiltersParentEnv(t *testing.T) {
	t.Setenv("TEST_DOCTOR_CANARY", "canary-leaked")
	stdout, _, err := collectDoctorOutput(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "fake-doctor-stdout" {
		t.Errorf("non-allowlisted env var reached the re-exec: stdout = %q", stdout)
	}
}
