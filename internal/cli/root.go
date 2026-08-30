// Package cli wires the cobra command tree and drives the top-level event
// loop; exit codes follow the taxonomy in docs/cli/exit-codes.md.
package cli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/version"
)

// cfgFile is cobra-managed state from --config; passing it as a param would fight cobra's pattern.
var cfgFile string

var (
	logLevel   string
	logFormat  string
	logFile    string
	logQuiet   bool
	logVerbose bool
)

// startLogged records that the "okdctl: started" bookend fired, keeping "finished" symmetric.
var startLogged bool

// preflightWarns holds pre-Execute warnings; PersistentPreRunE drains them
// after configureLogging so they use the final formatter.
var preflightWarns []func()

// DeferWarn enqueues fn for PersistentPreRunE to call after configureLogging,
// for warnings raised before cli.Execute (e.g. main.preflight).
func DeferWarn(fn func()) {
	preflightWarns = append(preflightWarns, fn)
}

var rootCmd = &cobra.Command{
	Use:   "okdctl",
	Short: "Provision OKD clusters on Proxmox VE",
	Long: `okdctl provisions OKD clusters on Proxmox VE from an interactive wizard.
It's for the homelab operator with one or two Proxmox nodes who wants a
real Kubernetes cluster without hand-rolling Terraform, Ignition, and
bootstrap glue.

Release builds check api.github.com for a newer release (at most once
per 24h, cached locally); set OKDCTL_NO_UPDATE_CHECK=1 to disable.`,
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if err := configureLogging(cmd); err != nil {
			return err
		}
		// logged here (not execute()) so it honors --quiet/--log-format/the
		// piped-stderr auto-switch, symmetric with "finished"
		logutil.Info("okdctl: started", logutil.LF("argv", logutil.RedactableArgv(os.Args[1:])))
		startLogged = true
		for _, fn := range preflightWarns {
			fn()
		}
		preflightWarns = nil
		return ensureRoot(cmd)
	},
}

// Execute wires the default slog logger, runs the cobra tree, flushes the log
// file, and exits with execute()'s code.
func Execute() {
	slog.SetDefault(logutil.SimpleLogger())
	wrapArgValidators(rootCmd)
	code := execute()
	if logFileCloser != nil {
		_ = logFileCloser.Close()
	}
	os.Exit(code)
}

// wrapArgValidators wraps each Args validator so violations surface as
// UsageError (exit 64) instead of cobra's exit-1; existing UsageErrors pass
// through unchanged.
func wrapArgValidators(cmd *cobra.Command) {
	if fn := cmd.Args; fn != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			err := fn(c, args)
			if err == nil {
				return nil
			}
			var usageErr *errtypes.UsageError
			if errors.As(err, &usageErr) {
				return err
			}
			return &errtypes.UsageError{Msg: err.Error(), Err: err}
		}
	}
	for _, c := range cmd.Commands() {
		wrapArgValidators(c)
	}
}

func execute() (code int) {
	tui.SetRunID(rand.Text())
	start := time.Now()
	defer func() {
		// gated on startLogged so help/version paths (which skip
		// PersistentPreRunE) don't log "finished" without "started"
		if !startLogged {
			return
		}
		logutil.Info(
			"okdctl: finished",
			logutil.LF("duration", time.Since(start).Round(time.Millisecond).String()),
			logutil.LF("exit_code", code),
		)
	}()

	// recovers panics into exit 70 (not Go's default 2, reserved for
	// ConfigError); registered after the bookend defer so it runs first on
	// unwind.
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		code = 70
		logutil.Error("internal error: panic recovered", logutil.LF("panic", fmt.Sprint(r)))
		stack := debug.Stack()
		fmt.Fprintf(os.Stderr, "panic: %v\n\n%s", r, stack)
		if runLogSink != nil {
			fmt.Fprintf(runLogSink, "panic: %v\n\n%s", r, stack)
		}
	}()

	// custom signal handling distinguishes SIGINT(130) from SIGTERM(143);
	// signal.NotifyContext would collapse them
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		// close after Stop so the receiver's !ok branch returns cleanly;
		// otherwise it blocks until process exit (bounded leak)
		close(sigCh)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var caughtSig atomic.Value // os.Signal
	go signalLoop(sigCh, cancel, &caughtSig, os.Exit)

	updateCh := version.BackgroundCheck(ctx)

	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		if sigCode, handled := signalExitCode(&caughtSig, err); handled {
			return sigCode
		}
		if shouldAnnounceFailure(err) {
			announceFailure(err)
		}
		return exitCodeFor(err)
	}

	printUpdateNotice(updateCh)
	return 0
}

// signalLoop is the two-strike handler: first signal cancels and warns, second
// exits 143/130 bypassing cleanup since the user asked for a hard kill.
func signalLoop(sigCh <-chan os.Signal, cancel context.CancelFunc, caughtSig *atomic.Value, exit func(int)) {
	// converts a panic here to exit 70 (like execute()'s recover), since an
	// unrecovered one would use Go's exit 2, reserved for ConfigError
	defer func() {
		if r := recover(); r != nil {
			logutil.Error("internal error: panic in signal handler", logutil.LF("panic", fmt.Sprint(r)))
			exit(70)
		}
	}()
	sig, ok := <-sigCh
	if !ok {
		return
	}
	caughtSig.Store(sig)
	cancel()
	logutil.Warn("shutdown in progress; press ctrl-c again to force quit")
	sig2, ok := <-sigCh
	if !ok {
		return
	}
	if sig2 == syscall.SIGTERM {
		exit(143)
		return
	}
	exit(130)
}

func printUpdateNotice(ch <-chan version.CheckResult) {
	if logQuiet || logFormat == outputJSON {
		return
	}
	var result version.CheckResult
	t := time.NewTimer(100 * time.Millisecond)
	defer t.Stop()
	select {
	case result = <-ch:
	case <-t.C:
		return
	}
	if result.LatestTag == "" {
		return
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, tui.WarningStyle.Render("update available:")+" "+
		tui.MutedStyle.Render(version.Version)+" → "+
		tui.HighlightStyle.Render(result.LatestTag))
	fmt.Fprintln(os.Stderr, tui.MutedStyle.Render("  to upgrade (sha256 + cosign verified):"))
	fmt.Fprintln(os.Stderr, tui.MutedStyle.Render("  curl -sSfL https://raw.githubusercontent.com/qxtaiba/okdctl/develop/scripts/install.sh | bash"))
}

// announceFailure renders the boxed ErrorSummary on a TTY, else logs "command
// failed" structured so RedactHandler can scrub credentials (stringifying err
// first would bypass it).
func announceFailure(err error) {
	if logutil.ProgressBarsEnabled() && !render.IsPresented(err) {
		fmt.Fprint(os.Stderr, render.ErrorSummary(err, exitCodeFor(err), logutil.RunID()))
		return
	}
	logutil.Error("command failed", logutil.LF("err", err))
	if runLogPath != "" {
		logutil.Info("full run log persisted; attach it to bug reports or run 'okdctl debug-bundle'",
			logutil.LF("path", runLogPath))
	}
}

// shouldAnnounceFailure reports whether to print "command failed", excluding
// errDoctorWarn/errPlanDrift since each already summarizes itself.
func shouldAnnounceFailure(err error) bool {
	return !errors.Is(err, errDoctorWarn) && !errors.Is(err, errPlanDrift)
}

// signalExitCode reports the exit code for a caught signal (130 SIGINT, 143
// SIGTERM); handled is false when none was caught.
func signalExitCode(caughtSig *atomic.Value, err error) (int, bool) {
	if caughtSig.Load() == nil {
		return 0, false
	}
	// only context.Canceled: the root ctx has no deadline, so accepting
	// DeadlineExceeded here would misattribute a poll timeout to the signal
	if !errors.Is(err, context.Canceled) {
		return 0, false
	}
	if sig, _ := caughtSig.Load().(os.Signal); sig == syscall.SIGTERM {
		return 143, true
	}
	return 130, true
}

// exitCodeFor maps err to the documented BSD-sysexits code. Precedence is
// sentinel > Config(2) > Network(3) > Cluster(4) > Auth(5) > Usage(64)
// regardless of wrap order; see docs/cli/exit-codes.md.
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errtypes.ErrConfigMissing) {
		return 66
	}
	if errors.Is(err, errtypes.ErrPullSecretInvalid) {
		return 65
	}
	if errors.Is(err, errtypes.ErrSudoMissing) {
		return 71
	}
	// errDoctorWarn/errPlanDrift are cli-local sentinels with their own codes,
	// not folded into ConfigError(2), so they stay distinguishable from real
	// failures
	if errors.Is(err, errDoctorWarn) {
		return 6
	}
	if errors.Is(err, errPlanDrift) {
		return 7
	}
	var cfgErr *errtypes.ConfigError
	if errors.As(err, &cfgErr) {
		return 2
	}
	var netErr *errtypes.NetworkError
	if errors.As(err, &netErr) {
		return 3
	}
	var clusterErr *errtypes.ClusterError
	if errors.As(err, &clusterErr) {
		return 4
	}
	var authErr *errtypes.AuthError
	if errors.As(err, &authErr) {
		return 5
	}
	var usageErr *errtypes.UsageError
	// 64 = EX_USAGE (BSD sysexits.h); returned via UsageError so deferred
	// closes run before process exit
	if errors.As(err, &usageErr) {
		return 64
	}
	return 1
}

// versionOutput is the machine-readable shape for "okdctl version
// --output=json"; fields match internal/version/version.go's ldflags vars.
type versionOutput struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

var versionOutputFlag string

// versionCmd exists so "okdctl version" works alongside "okdctl --version"
// (kubectl/docker/gh pattern).
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, git commit, build date",
	Long: `Print the okdctl build identity: version number, git commit SHA,
build date, Go toolchain version, and OS/arch platform.

Pass --output=json for machine-readable output suitable for CI version
pinning or scripted comparisons (see docs/cli/json-schema.md).`,
	Example: `  okdctl version
  okdctl version --output json | jq .version`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateFormat(versionOutputFlag); err != nil {
			return err
		}
		quietForJSON(versionOutputFlag)
		if versionOutputFlag == outputJSON {
			return writeJSON(cmd.OutOrStdout(), versionOutput{
				Version:   version.Version,
				GitCommit: version.GitCommit,
				BuildDate: version.BuildDate,
				GoVersion: version.GoVersion,
				Platform:  version.Platform,
			})
		}
		_, err := fmt.Fprint(cmd.OutOrStdout(), versionText())
		return err
	},
}

// versionText renders build identity with okdctl's dotted-leader convention
// instead of cobra's stock "Key:" colons.
func versionText() string {
	const keyCol = 16
	rows := [][2]string{
		{"git commit", version.GitCommit},
		{"build date", version.BuildDate},
		{"go version", version.GoVersion},
		{"platform", version.Platform},
	}
	var b strings.Builder
	fmt.Fprintf(&b, "okdctl %s\n", version.Version)
	for _, r := range rows {
		b.WriteString("  " + tui.DottedKeyValueFull(r[0], r[1], keyCol, 0) + "\n")
	}
	return b.String()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, flagConfig, flagConfigShort, "okdctl.yaml", "configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, flagLogLevel, "info", "log verbosity (debug, info, warn, error)")
	_ = rootCmd.RegisterFlagCompletionFunc(flagLogLevel,
		cobra.FixedCompletions([]string{"debug", "info", "warn", "error"}, cobra.ShellCompDirectiveNoFileComp))
	rootCmd.PersistentFlags().StringVar(&logFormat, flagLogFormat, "text", "log output format: text (TTY default) | json (auto-selected when stderr is piped)")
	_ = rootCmd.RegisterFlagCompletionFunc(flagLogFormat,
		cobra.FixedCompletions([]string{outputText, outputJSON}, cobra.ShellCompDirectiveNoFileComp))
	// DefValue is blanked so --help doesn't print '(default "text")',
	// contradicting the auto-switch prose; keep in sync with the flag's Usage
	// string
	rootCmd.PersistentFlags().Lookup(flagLogFormat).DefValue = ""
	rootCmd.PersistentFlags().StringVar(&logFile, flagLogFile, "", "write log output to this file in addition to stderr (replaces the default okdctl.log sink of deploy/destroy/cleanup)")
	rootCmd.PersistentFlags().BoolVarP(&logQuiet, flagQuiet, "q", false, "suppress info/warn logs (alias for --log-level=error)")
	rootCmd.PersistentFlags().BoolVarP(&logVerbose, flagVerbose, "v", false, "enable debug logging (alias for --log-level=debug)")
	rootCmd.MarkFlagsMutuallyExclusive(flagQuiet, flagVerbose)

	// returns UsageError instead of os.Exit so Execute's deferred logFileCloser.Close() still runs
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		logutil.Error("flag error", logutil.LF("err", err))
		return &errtypes.UsageError{Msg: err.Error(), Err: err}
	})

	versionCmd.Flags().StringVarP(&versionOutputFlag, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(versionCmd)

	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(updateIngressCmd)
	rootCmd.AddCommand(versionCmd)

	rootCmd.SetVersionTemplate(versionText())
}
