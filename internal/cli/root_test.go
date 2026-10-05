package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
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

// TestHighestTrafficCommandsCarryExamples guards E-C7: root plus the
// highest-traffic leaves each carry a non-empty Example, so `--help` shows
// an EXAMPLES section (installHelp's usage template only renders one when
// .HasExample is true) — node resize is the model this list follows.
func TestHighestTrafficCommandsCarryExamples(t *testing.T) {
	want := []string{
		"okdctl",
		"okdctl deploy",
		"okdctl destroy",
		"okdctl status",
		"okdctl node manage",
		"okdctl config validate",
	}
	for _, path := range want {
		t.Run(path, func(t *testing.T) {
			args := strings.Fields(path)[1:] // drop the leading "okdctl"
			cmd, _, err := rootCmd.Find(args)
			if err != nil {
				t.Fatalf("Find(%v) = %v", args, err)
			}
			if strings.TrimSpace(cmd.Example) == "" {
				t.Errorf("%s has no Example; --help would show no EXAMPLES section", path)
			}
		})
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
			// mainDone never closes: these cases exercise a hung (or
			// nonexistent, for the single-signal case) main goroutine, so
			// the second signal must fall through the grace bound.
			mainDone := make(chan struct{})

			signalLoop(sigCh, cancel, &caughtSig, func(code int) { exitCode = code }, mainDone, time.Millisecond)

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

// TestSignalLoop_SecondSignalAwaitsCleanup proves a second real signal no
// longer skips execute()'s deferred credential Zeroize/ZeroizeEnv and
// model.shutdown — it waits for mainDone (ExecuteContext having returned,
// meaning that cleanup already ran) before deciding whether to force exit.
func TestSignalLoop_SecondSignalAwaitsCleanup(t *testing.T) {
	t.Run("cleanup finishes within grace: no force exit", func(t *testing.T) {
		sigCh := make(chan os.Signal, 2)
		_, cancel := context.WithCancel(context.Background())
		var caughtSig atomic.Value
		exitCode := -1
		sigCh <- syscall.SIGINT
		sigCh <- syscall.SIGINT
		close(sigCh)
		mainDone := make(chan struct{})
		close(mainDone) // ExecuteContext already returned; cleanup already ran.

		signalLoop(sigCh, cancel, &caughtSig, func(code int) { exitCode = code }, mainDone, time.Hour)

		if exitCode != -1 {
			t.Fatalf("exit code = %d, want no forced exit once cleanup already finished", exitCode)
		}
	})

	t.Run("cleanup hangs past grace: forces exit with the signal's code", func(t *testing.T) {
		sigCh := make(chan os.Signal, 2)
		_, cancel := context.WithCancel(context.Background())
		var caughtSig atomic.Value
		exitCode := -1
		sigCh <- syscall.SIGTERM
		sigCh <- syscall.SIGTERM
		close(sigCh)
		mainDone := make(chan struct{}) // never closes: cleanup is still hung.

		signalLoop(sigCh, cancel, &caughtSig, func(code int) { exitCode = code }, mainDone, time.Millisecond)

		if exitCode != 143 {
			t.Fatalf("exit code = %d, want 143 once the grace bound is exceeded", exitCode)
		}
	})
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

// TestShouldRenderErrorBox guards item 7 of the second-cut safety findings:
// colorOff (NO_COLOR/--no-color) used to flip progressBars, which
// announceFailure gated on, so NO_COLOR degraded the boxed error to the
// flat "[ERROR]" line — box drawing is structure, not color. The gate keeps
// its TTY/json/presented checks but takes no color signal at all, so this
// table exhaustively covers stderr/stdout TTY-ness and format without ever
// mentioning NO_COLOR — its absence from the signature is the fix.
func TestShouldRenderErrorBox(t *testing.T) {
	plain := errors.New("boom")
	cases := []struct {
		name                 string
		stderrTTY, stdoutTTY bool
		format               string
		err                  error
		want                 bool
	}{
		{"both TTY, text format renders the box", true, true, tui.FormatText, plain, true},
		{"piped stderr gets the flat line", false, true, tui.FormatText, plain, false},
		{"piped stdout gets the flat line", true, false, tui.FormatText, plain, false},
		{"json format gets the flat line", true, true, tui.FormatJSON, plain, false},
		{"already-presented error gets the flat line", true, true, tui.FormatText, render.Presented(plain), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRenderErrorBox(tc.stderrTTY, tc.stdoutTTY, tc.format, tc.err); got != tc.want {
				t.Errorf("shouldRenderErrorBox(%v, %v, %q, err) = %v, want %v", tc.stderrTTY, tc.stdoutTTY, tc.format, got, tc.want)
			}
		})
	}
}

// TestErrorSummaryANSIFreeUnderNoColor proves the other half of item 7's
// claim: once shouldRenderErrorBox lets a NO_COLOR run through, the box it
// renders is genuinely color-free (Downsampled) while still carrying its
// box-drawing structure — NO_COLOR strips color, not the box.
func TestErrorSummaryANSIFreeUnderNoColor(t *testing.T) {
	tui.SetColorProfileFor(&bytes.Buffer{}) // a buffer is never a TTY
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })

	out := render.ErrorSummary(&errtypes.ConfigError{Msg: "bad yaml"}, 2, "run-123")

	if strings.Contains(out, "\x1b[") {
		t.Errorf("ErrorSummary leaked ANSI escapes under a no-color profile:\n%q", out)
	}
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╯") {
		t.Errorf("ErrorSummary must still draw its box structure under NO_COLOR:\n%s", out)
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

func installLogBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logutil.InstallHandler(slog.NewTextHandler(&buf, nil))
	t.Cleanup(func() { logutil.InstallHandler(slog.NewTextHandler(os.Stderr, nil)) })
	return &buf
}

func TestFlagErrorFuncReturnsUsageErrorWithHelpHint(t *testing.T) {
	buf := installLogBuffer(t)

	err := rootCmd.FlagErrorFunc()(nodeResizeCmd, errors.New("unknown flag: --bogus"))

	var usageErr *errtypes.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("want *errtypes.UsageError, got %T: %v", err, err)
	}
	if got := exitCodeFor(err); got != 64 {
		t.Fatalf("exitCodeFor = %d, want 64", got)
	}
	d, ok := errtypes.Describe(err)
	if !ok {
		t.Fatalf("errtypes.Describe failed to classify %v", err)
	}
	if !strings.Contains(d.Hint, "okdctl node resize --help") {
		t.Fatalf("hint = %q, want it to contain %q", d.Hint, "okdctl node resize --help")
	}
	if buf.Len() != 0 {
		t.Fatalf("FlagErrorFunc must not log directly; buffer = %q", buf.String())
	}
}

func TestNoColorFlagIsLongFormOnly(t *testing.T) {
	f := rootCmd.PersistentFlags().Lookup(flagNoColor)
	if f == nil {
		t.Fatal("--no-color flag not registered")
	}
	if f.Shorthand != "" {
		t.Fatalf("--no-color has shorthand %q, want none (shorthand allowlist is closed)", f.Shorthand)
	}
}

// Regression guard: the bare "okdctl --version" flag short-circuits inside
// cobra's execute() before PersistentPreRunE/configureLogging ever runs, so
// --no-color must be honored by versionText itself.
func TestVersionFlagRespectsNoColor(t *testing.T) {
	// registered before t.Setenv so LIFO cleanup restores CLICOLOR_FORCE
	// first and only then re-detects the profile with a clean environment;
	// the reverse order left the package profile forced-colourful for later
	// tests
	t.Cleanup(func() { tui.SetColorProfileFor(&bytes.Buffer{}) })
	t.Setenv("CLICOLOR_FORCE", "1") // forces color even for a non-TTY writer

	tui.SetColorProfileFor(&bytes.Buffer{})

	if got := tui.Downsample(tui.SuccessStyle.Render("x")); !strings.Contains(got, "\x1b[") {
		t.Fatalf("test setup failed to force a colourful profile: %q", got)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"--no-color", "--version"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetArgs(nil)
		noColor = false
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute(--no-color --version) = %v", err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Errorf("--version leaked ANSI under --no-color: %q", out.String())
	}
}

func TestMutatesStateClassification(t *testing.T) {
	cases := []struct {
		cmd  *cobra.Command
		want bool
	}{
		{deployCmd, true},
		{destroyCmd, true},
		{cleanupCmd, true},
		{updateIngressCmd, true},
		{nodeAddCmd, true},
		{nodeRemoveCmd, true},
		{nodeResizeCmd, true},
		{clusterStopCmd, true},
		{addonUninstallCmd, true},
		{versionCmd, false},
		{statusCmd, false},
		{nodeListCmd, false},
		{releasesListCmd, false},
		{addonListCmd, false},
		{addonVerifyCmd, false},
		{doctorCmd, false},
		{planCmd, false},
	}
	for _, tc := range cases {
		if got := mutatesState(tc.cmd); got != tc.want {
			t.Errorf("mutatesState(%s) = %v, want %v", tc.cmd.CommandPath(), got, tc.want)
		}
	}
}
