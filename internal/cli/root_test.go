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
	"github.com/spf13/pflag"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// tripwire: pflag embeds flag values in error text (unscrubbed UsageError.Msg);
// no credential-named flag without scrubbing first.
func TestNoRegisteredFlagNameLooksLikeCredential(t *testing.T) {
	var offenders []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		check := func(f *pflag.Flag) {
			if logutil.KeyIsSecret(f.Name) {
				offenders = append(offenders, c.CommandPath()+" --"+f.Name)
			}
		}
		c.Flags().VisitAll(check)
		c.PersistentFlags().VisitAll(check)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
	if len(offenders) > 0 {
		t.Errorf("credential-named flag(s) registered; pflag embeds flag values in "+
			"error text that becomes UsageError.Msg unscrubbed — scrub before Msg "+
			"or rename: %v", offenders)
	}
}

func TestSignalLoop(t *testing.T) {
	cases := []struct {
		name     string
		sigs     []os.Signal
		wantExit int // -1 means exit must not be called
	}{
		{"first signal cancels without exit", []os.Signal{syscall.SIGINT}, -1},
		{"second SIGINT forces exit 130", []os.Signal{syscall.SIGINT, syscall.SIGINT}, 130},
		{"second SIGTERM forces exit 143", []os.Signal{syscall.SIGTERM, syscall.SIGTERM}, 143},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sigCh := make(chan os.Signal, 2)
			ctx, cancel := context.WithCancel(context.Background())
			var caughtSig atomic.Value
			exitCode := -1
			for _, sig := range tc.sigs {
				sigCh <- sig
			}
			close(sigCh)

			signalLoop(sigCh, cancel, &caughtSig, func(code int) { exitCode = code })

			if ctx.Err() == nil {
				t.Fatal("expected context to be canceled after first signal")
			}
			if caughtSig.Load() == nil {
				t.Fatal("caughtSig should be stored after first signal")
			}
			if exitCode != tc.wantExit {
				t.Fatalf("exit code = %d, want %d", exitCode, tc.wantExit)
			}
		})
	}
}

// pins the exit-code contract; external scripts depend on this mapping, so a
// change here is a user-facing break.
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
		// ClusterError wrapping a context sentinel must resolve to 4;
		// execute()'s caughtSig gate ensures errors.Is only short-circuits on a
		// real signal
		{"ClusterErrorWrapsDeadline", &errtypes.ClusterError{Msg: "budget", Err: context.DeadlineExceeded}, 4},
		{"ClusterErrorWrapsCanceled", &errtypes.ClusterError{Msg: "canceled", Err: context.Canceled}, 4},
		{"UsageError", &errtypes.UsageError{Msg: "unknown flag"}, 64},
		// Granular BSD sysexits sentinels: specific code beats broad category.
		{"ErrConfigMissing_direct", &errtypes.ConfigError{Msg: "not found", Err: errtypes.ErrConfigMissing}, 66},
		{"ErrConfigMissing_wrapped", fmt.Errorf("load: %w", &errtypes.ConfigError{Msg: "not found", Err: errtypes.ErrConfigMissing}), 66},
		{"ErrPullSecretInvalid_direct", &errtypes.AuthError{Msg: "bad json", Err: errtypes.ErrPullSecretInvalid}, 65},
		{"ErrSudoMissing_direct", &errtypes.AuthError{Msg: "no sudo", Err: errtypes.ErrSudoMissing}, 71},
		// doctor's warn-only sentinel is cli-local (not an errtypes category); see errDoctorWarn
		{"errDoctorWarn_direct", errDoctorWarn, 6},
		{"errDoctorWarn_wrapped", fmt.Errorf("doctor: %w", errDoctorWarn), 6},
		// plan's drift-found sentinel mirrors errDoctorWarn (cli-local); see errPlanDrift
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
			// a poll timeout racing a caught signal keeps its own code
			// (ClusterError→4): the root ctx is WithCancel, so a real signal
			// only ever surfaces as Canceled
			name:        "SIGINT ClusterError wrapping DeadlineExceeded falls through",
			caughtSig:   storeSignal(syscall.SIGINT),
			err:         &errtypes.ClusterError{Msg: "budget", Err: context.DeadlineExceeded},
			wantCode:    0,
			wantHandled: false,
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
