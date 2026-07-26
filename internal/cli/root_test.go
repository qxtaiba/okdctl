package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// TestSignalLoopFirstSignalCancels verifies that a single signal cancels the
// context and stores caughtSig without calling exit.
func TestSignalLoopFirstSignalCancels(t *testing.T) {
	sigCh := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())
	var caughtSig atomic.Value
	exitCalled := false
	fakeExit := func(_ int) { exitCalled = true }

	sigCh <- syscall.SIGINT
	close(sigCh)
	signalLoop(sigCh, cancel, &caughtSig, fakeExit)

	if ctx.Err() == nil {
		t.Fatal("expected context to be canceled after first signal")
	}
	if exitCalled {
		t.Fatal("exit should not be called when only one signal is delivered")
	}
	if caughtSig.Load() == nil {
		t.Fatal("caughtSig should be stored after first signal")
	}
}

// TestSignalLoopSecondSignalForcesExit verifies that a second signal triggers
// exit(130) immediately.
func TestSignalLoopSecondSignalForcesExit(t *testing.T) {
	sigCh := make(chan os.Signal, 2)
	_, cancel := context.WithCancel(context.Background())
	var caughtSig atomic.Value
	exitCode := -1
	fakeExit := func(code int) { exitCode = code }

	sigCh <- syscall.SIGINT
	sigCh <- syscall.SIGINT
	signalLoop(sigCh, cancel, &caughtSig, fakeExit)

	if exitCode != 130 {
		t.Fatalf("expected exit(130) on second signal, got exit(%d)", exitCode)
	}
}

// TestSignalLoopSecondSignalSIGTERMForces143 verifies that a second SIGTERM
// exits 143 (not the SIGINT-shaped 130), matching the documented taxonomy.
func TestSignalLoopSecondSignalSIGTERMForces143(t *testing.T) {
	sigCh := make(chan os.Signal, 2)
	_, cancel := context.WithCancel(context.Background())
	var caughtSig atomic.Value
	exitCode := -1
	fakeExit := func(code int) { exitCode = code }

	sigCh <- syscall.SIGTERM
	sigCh <- syscall.SIGTERM
	signalLoop(sigCh, cancel, &caughtSig, fakeExit)

	if exitCode != 143 {
		t.Fatalf("expected exit(143) on second SIGTERM, got exit(%d)", exitCode)
	}
}

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
		{"nilIsSuccess", nil, 0},
		// ClusterError wrapping a context sentinel must resolve to 4 via
		// exitCodeFor. The caughtSig gate in execute() ensures the
		// errors.Is short-circuit fires only when a real signal was caught.
		{"ClusterErrorWrapsDeadline", &errtypes.ClusterError{Msg: "budget", Err: context.DeadlineExceeded}, 4},
		{"ClusterErrorWrapsCanceled", &errtypes.ClusterError{Msg: "canceled", Err: context.Canceled}, 4},
		{"UsageError", &errtypes.UsageError{Msg: "unknown flag"}, 64},
		// Granular BSD sysexits sentinels: specific code beats broad category.
		{"ErrConfigMissing_direct", &errtypes.ConfigError{Msg: "not found", Err: errtypes.ErrConfigMissing}, 66},
		{"ErrConfigMissing_wrapped", fmt.Errorf("load: %w", &errtypes.ConfigError{Msg: "not found", Err: errtypes.ErrConfigMissing}), 66},
		{"ErrPullSecretInvalid_direct", &errtypes.AuthError{Msg: "bad json", Err: errtypes.ErrPullSecretInvalid}, 65},
		{"ErrSudoMissing_direct", &errtypes.AuthError{Msg: "no sudo", Err: errtypes.ErrSudoMissing}, 71},
		// doctor's warn-only sentinel is cli-local (not an errtypes category)
		// and carries its own dedicated code; see errDoctorWarn.
		{"errDoctorWarn_direct", errDoctorWarn, 6},
		{"errDoctorWarn_wrapped", fmt.Errorf("doctor: %w", errDoctorWarn), 6},
		// plan's drift-found sentinel mirrors errDoctorWarn: cli-local, its
		// own dedicated code; see errPlanDrift.
		{"errPlanDrift_direct", errPlanDrift, 7},
		{"errPlanDrift_wrapped", fmt.Errorf("plan: %w", errPlanDrift), 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCodeFor(tc.err); got != tc.want {
				t.Fatalf("exitCodeFor(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestShouldAnnounceFailure locks the one exception to execute()'s generic
// "command failed" line: a warn-only doctor run (errDoctorWarn) is not an
// actual failure, so it must not trigger the announcement.
func TestShouldAnnounceFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"errDoctorWarn suppressed", errDoctorWarn, false},
		{"wrapped errDoctorWarn suppressed", fmt.Errorf("doctor: %w", errDoctorWarn), false},
		{"errPlanDrift suppressed", errPlanDrift, false},
		{"wrapped errPlanDrift suppressed", fmt.Errorf("plan: %w", errPlanDrift), false},
		{"ConfigError announced", &errtypes.ConfigError{Msg: "bad yaml"}, true},
		{"generic error announced", errors.New("boom"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAnnounceFailure(tc.err); got != tc.want {
				t.Errorf("shouldAnnounceFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSignalExitCode(t *testing.T) {
	storeSignal := func(sig os.Signal) *atomic.Value {
		var v atomic.Value
		v.Store(sig)
		return &v
	}
	var empty atomic.Value

	cases := []struct {
		name        string
		caughtSig   *atomic.Value
		err         error
		wantCode    int
		wantHandled bool
	}{
		{
			name:        "no signal DeadlineExceeded falls through",
			caughtSig:   &empty,
			err:         context.DeadlineExceeded,
			wantCode:    0,
			wantHandled: false,
		},
		{
			name:        "SIGINT Canceled returns 130",
			caughtSig:   storeSignal(syscall.SIGINT),
			err:         context.Canceled,
			wantCode:    130,
			wantHandled: true,
		},
		{
			name:        "SIGTERM Canceled returns 143",
			caughtSig:   storeSignal(syscall.SIGTERM),
			err:         context.Canceled,
			wantCode:    143,
			wantHandled: true,
		},
		{
			name:        "SIGINT ClusterError wrapping DeadlineExceeded returns 130",
			caughtSig:   storeSignal(syscall.SIGINT),
			err:         &errtypes.ClusterError{Msg: "budget", Err: context.DeadlineExceeded},
			wantCode:    130,
			wantHandled: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, handled := signalExitCode(tc.caughtSig, tc.err)
			if handled != tc.wantHandled {
				t.Fatalf("handled=%v, want %v", handled, tc.wantHandled)
			}
			if handled && code != tc.wantCode {
				t.Fatalf("code=%d, want %d", code, tc.wantCode)
			}
		})
	}
}

// TestExecutePanicExitsSoftware locks the crash contract: a panic anywhere
// under the cobra tree must exit 70 (EX_SOFTWARE), not the Go runtime's own
// exit 2, which the published taxonomy reserves for ConfigError.
func TestExecutePanicExitsSoftware(t *testing.T) {
	t.Setenv("OKDCTL_NO_UPDATE_CHECK", "1")
	t.Setenv(wizardDemoEnv, "1")

	panicCmd := &cobra.Command{
		Use:    "panic-test",
		Hidden: true,
		RunE:   func(*cobra.Command, []string) error { panic("boom") },
	}
	rootCmd.AddCommand(panicCmd)
	rootCmd.SetArgs([]string{"panic-test"})
	t.Cleanup(func() {
		rootCmd.RemoveCommand(panicCmd)
		rootCmd.SetArgs(nil)
	})

	if got := execute(); got != 70 {
		t.Fatalf("execute() after panic = %d, want 70 (EX_SOFTWARE)", got)
	}
}

// TestWrapArgValidators locks the arg-count exit-code policy: cobra's bare
// validator errors become UsageError (exit 64), hand-rolled UsageError
// validators pass through unwrapped, and valid invocations are untouched.
func TestWrapArgValidators(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	exact := &cobra.Command{Use: "exact", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { return nil }}
	handRolled := &cobra.Command{Use: "hand", Args: func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return &errtypes.UsageError{Msg: "expected exactly one name"}
		}
		return nil
	}}
	root.AddCommand(exact)
	root.AddCommand(handRolled)
	wrapArgValidators(root)

	if err := exact.Args(exact, []string{"one"}); err != nil {
		t.Fatalf("valid arg count must pass: %v", err)
	}

	err := exact.Args(exact, nil)
	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("cobra arg-count violation must map to UsageError (exit 64), got %T: %v", err, err)
	}
	if exitCodeFor(err) != 64 {
		t.Fatalf("exitCodeFor(wrapped arg-count error) = %d, want 64", exitCodeFor(err))
	}

	err = handRolled.Args(handRolled, nil)
	if !errors.As(err, &usageErr) || usageErr.Msg != "expected exactly one name" {
		t.Fatalf("hand-rolled UsageError must pass through unwrapped, got %v", err)
	}
}
