// Package cli wires together the cobra command tree and drives the
// top-level event loop. Process exit codes follow a documented contract:
// config error=2, network error=3, cluster error=4, auth error=5
// (includes invoked-as-root rejection via AuthError), config file not
// found=66 (EX_NOINPUT), invalid pull secret JSON=65 (EX_DATAERR),
// sudo not found=71 (EX_OSERR), unknown-flag error=64 (EX_USAGE, via
// SetFlagErrorFunc), other error=1 (includes unknown subcommands,
// arg-count violations, and mutually-exclusive-flag conflicts which cobra
// surfaces outside the flag-parser), SIGINT=130, SIGTERM=143, success=0.
// See docs/cli/exit-codes.md for the full taxonomy table.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/version"
)

// cfgFile is package-scope state managed by cobra PersistentFlags. It is
// populated once by the root command's --config flag and read directly by
// config-consuming subcommand RunE handlers. This is the standard cobra
// pattern; threading it through function parameters would fight the
// framework.
var cfgFile string

var (
	logLevel   string
	logFormat  string
	logFile    string
	logQuiet   bool
	logVerbose bool
)

// preflightWarns holds warning closures registered before cli.Execute
// runs. PersistentPreRunE drains the slice after configureLogging so every
// warning uses the fully-configured formatter (text or JSON).
var preflightWarns []func()

// DeferWarn enqueues fn to be called by PersistentPreRunE after
// configureLogging completes. Use this for warnings generated before
// cli.Execute is invoked (e.g. in main.preflight).
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
		for _, fn := range preflightWarns {
			fn()
		}
		preflightWarns = nil
		return ensureRoot(cmd)
	},
}

// Execute is the process-level entry point. It wires the default slog logger,
// runs the cobra tree, flushes any log file, and exits with the exit code
// computed by execute().
func Execute() {
	slog.SetDefault(tui.SimpleLogger())
	code := execute()
	if logFileCloser != nil {
		_ = logFileCloser.Close()
	}
	os.Exit(code)
}

func execute() (code int) {
	tui.SetRunID(system.NewUUIDv4())
	start := time.Now()
	tui.Info("okdctl: started", tui.LF("argv", logutil.RedactableArgv(os.Args[1:])))
	defer func() {
		tui.Info("okdctl: finished",
			tui.LF("duration", time.Since(start).Round(time.Millisecond).String()),
			tui.LF("exit_code", code),
		)
	}()

	// Roll our own signal handling so we can tell SIGINT (→130) apart from
	// SIGTERM (→143). signal.NotifyContext would collapse them.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		// Close after Stop so the receiver's !ok branch returns on the
		// happy path. Without this, the goroutine blocks on sigCh until
		// process exit — a bounded leak but still a leak.
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
		// Pass err as a structured attr so logutil.RedactHandler gets the
		// chance to scrub credentials in the chain. tui.Error(err.Error())
		// would stringify before the handler sees it.
		tui.Error("command failed", tui.LF("err", err))
		if runLogPath != "" {
			tui.Info("full run log persisted; attach it to bug reports or run 'okdctl debug-bundle'",
				tui.LF("path", runLogPath))
		}
		return exitCodeFor(err)
	}

	printUpdateNotice(updateCh)
	return 0
}

// signalLoop runs the two-strike shutdown handler in execute()'s goroutine.
// First signal: store, cancel, and print the escape-hatch hint. Second
// signal: exit(143) if it was SIGTERM, exit(130) otherwise — matching the
// documented taxonomy (130=SIGINT, 143=SIGTERM). The user has explicitly
// asked for a hard kill, so deferred cleanup (logFileCloser.Close) is
// intentionally bypassed. On the happy path execute()'s defer close(sigCh)
// fires after signal.Stop, causing the second receive to observe !ok and
// return cleanly (bounded goroutine leak).
// close(sigCh) ordering after signal.Stop is load-bearing for the
// bounded-leak contract; do not move or remove without re-deriving it.
func signalLoop(sigCh <-chan os.Signal, cancel context.CancelFunc, caughtSig *atomic.Value, exit func(int)) {
	sig, ok := <-sigCh
	if !ok {
		return
	}
	caughtSig.Store(sig)
	cancel()
	tui.Warn("shutdown in progress; press ctrl-c again to force quit")
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

// signalExitCode reports whether err was caused by a caught OS signal and,
// if so, returns the corresponding exit code (130 for SIGINT, 143 for SIGTERM).
// When no signal was received, handled is false and the caller resolves the
// exit code via exitCodeFor.
func signalExitCode(caughtSig *atomic.Value, err error) (int, bool) {
	if caughtSig.Load() == nil {
		return 0, false
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return 0, false
	}
	if sig, _ := caughtSig.Load().(os.Signal); sig == syscall.SIGTERM {
		return 143, true
	}
	return 130, true
}

// exitCodeFor maps err to the documented BSD-sysexits exit code. Sentinel
// errors (66/65/71) outrank every category below, and within categories the
// check order below is the precedence order: a category type found anywhere
// in the error chain wins over a different category type wrapping it,
// regardless of which is the outermost wrap. Precedence is Config(2) >
// Network(3) > Cluster(4) > Auth(5) > Usage(64). See docs/cli/exit-codes.md.
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	// Granular BSD sysexits sentinels take precedence over the broad typed
	// error categories below; a sentinel wrapped inside a ConfigError or
	// AuthError must resolve to the specific code, not the category code.
	if errors.Is(err, errtypes.ErrConfigMissing) {
		return 66
	}
	if errors.Is(err, errtypes.ErrPullSecretInvalid) {
		return 65
	}
	if errors.Is(err, errtypes.ErrSudoMissing) {
		return 71
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
	// 64 = EX_USAGE (BSD sysexits.h): command-line usage error. Returned by
	// SetFlagErrorFunc via UsageError so deferred closes run before exit.
	if errors.As(err, &usageErr) {
		return 64
	}
	return 1
}

// versionOutput is the machine-readable shape emitted by
// `okdctl version --output=json`. Field names match the ldflags variables
// in internal/version/version.go; see docs/cli/json-schema.md.
type versionOutput struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

var versionOutputFlag string

// versionCmd prints the same template as the --version flag. Exists so
// `okdctl version` works alongside `okdctl --version` (kubectl/docker/gh
// all expose both).
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, git commit, build date",
	Long: `Print the okdctl build identity: version number, git commit SHA,
build date, Go toolchain version, and OS/arch platform.

Pass --output=json for machine-readable output suitable for CI version
pinning or scripted comparisons (see docs/cli/json-schema.md).`,
	Example: `  okdctl version
  okdctl version --output json | jq .version`,
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
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "okdctl %s\nGit Commit: %s\nBuild Date: %s\nGo Version: %s\nPlatform:   %s\n",
			version.Version, version.GitCommit, version.BuildDate, version.GoVersion, version.Platform)
		return err
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, flagConfig, flagConfigShort, "okdctl.yaml", "configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, flagLogLevel, "info", "log verbosity (debug, info, warn, error)")
	_ = rootCmd.RegisterFlagCompletionFunc(flagLogLevel,
		cobra.FixedCompletions([]string{"debug", "info", "warn", "error"}, cobra.ShellCompDirectiveNoFileComp))
	rootCmd.PersistentFlags().StringVar(&logFormat, flagLogFormat, "text", "log output format: text (TTY default) | json (auto-selected when stderr is piped)")
	_ = rootCmd.RegisterFlagCompletionFunc(flagLogFormat,
		cobra.FixedCompletions([]string{outputText, outputJSON}, cobra.ShellCompDirectiveNoFileComp))
	// DefValue is blanked so --help does not print '(default "text")', which
	// would contradict the auto-switch prose above. Do not remove without also
	// updating the flag's Usage string to describe the TTY-vs-pipe contract.
	rootCmd.PersistentFlags().Lookup(flagLogFormat).DefValue = ""
	rootCmd.PersistentFlags().StringVar(&logFile, flagLogFile, "", "write log output to this file in addition to stderr (replaces the default okdctl.log sink of deploy/destroy/cleanup)")
	rootCmd.PersistentFlags().BoolVarP(&logQuiet, flagQuiet, "q", false, "suppress info/warn logs (alias for --log-level=error)")
	rootCmd.PersistentFlags().BoolVarP(&logVerbose, flagVerbose, "v", false, "enable debug logging (alias for --log-level=debug)")
	rootCmd.MarkFlagsMutuallyExclusive(flagQuiet, flagVerbose)

	// Return UsageError instead of os.Exit so Execute's deferred
	// logFileCloser.Close() runs before the process exits.
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		tui.Error("flag error", tui.LF("err", err))
		return &errtypes.UsageError{Msg: err.Error(), Err: err}
	})

	versionCmd.Flags().StringVarP(&versionOutputFlag, flagOutput, flagOutputShort, outputText, "output format: text|json")
	registerOutputCompletion(versionCmd)

	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(updateIngressCmd)
	rootCmd.AddCommand(versionCmd)

	rootCmd.SetVersionTemplate(fmt.Sprintf(`{{with .Name}}{{printf "%%s " .}}{{end}}{{printf "%%s" .Version}}
Git Commit: %s
Build Date: %s
Go Version: %s
Platform:   %s
`, version.GitCommit, version.BuildDate, version.GoVersion, version.Platform))
}
