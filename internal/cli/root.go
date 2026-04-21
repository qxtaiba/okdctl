// Package cli wires together the cobra command tree and drives the
// top-level event loop. Process exit codes follow a documented contract:
// config error=2, network error=3, cluster error=4, auth error=5,
// unknown-flag error=64 (EX_USAGE, via SetFlagErrorFunc), other error=1
// (includes unknown subcommands, arg-count violations, and mutually-
// exclusive-flag conflicts which cobra surfaces outside the flag-parser),
// SIGINT=130, SIGTERM=143, success=0.
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
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/version"
)

// cfgFile is package-scope state managed by cobra PersistentFlags. It is
// populated once by the root command's --config flag and read by subcommand
// RunE handlers (deploy, destroy, update-ingress) via direct package
// reference. This is the standard cobra pattern; threading it through
// function parameters would fight the framework.
var cfgFile string

var (
	logLevel   string
	logFormat  string
	logFile    string
	logQuiet   bool
	logVerbose bool
)

var rootCmd = &cobra.Command{
	Use:   "okdctl",
	Short: "Deploy production-ready Kubernetes clusters",
	Long: `Homelab K8s - Deploy production-ready Kubernetes clusters

A delightful CLI tool for deploying OKD/OpenShift clusters
on Proxmox VE infrastructure.

Highlights:
  • Interactive setup wizard with beautiful TUI
  • OKD/OpenShift 4.15-4.21 support
  • Addon-extensible architecture (Flux, secrets, storage, cert-manager)
  • YAML configuration with sensible defaults
  • Automated preflight checks and validation
  • Single binary distribution`,
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if err := configureLogging(); err != nil {
			return err
		}
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

func execute() int {
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
	go func() {
		sig, ok := <-sigCh
		if !ok {
			return
		}
		caughtSig.Store(sig)
		cancel()
	}()

	updateCh := version.BackgroundCheck(ctx)

	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			if sig, _ := caughtSig.Load().(os.Signal); sig == syscall.SIGTERM {
				return 143
			}
			return 130
		}
		// Pass err as a structured attr so logutil.RedactHandler gets the
		// chance to scrub credentials in the chain. tui.Error(err.Error())
		// would stringify before the handler sees it.
		tui.Error("command failed", tui.LF("err", err))
		return exitCodeFor(err)
	}

	printUpdateNotice(updateCh)
	return 0
}

func printUpdateNotice(ch <-chan version.CheckResult) {
	var result version.CheckResult
	select {
	case result = <-ch:
	case <-time.After(100 * time.Millisecond):
		return
	}
	if result.LatestTag == "" {
		return
	}
	fmt.Println()
	fmt.Println(tui.WarningStyle.Render("update available:") + " " +
		tui.MutedStyle.Render(version.Version) + " → " +
		tui.HighlightStyle.Render(result.LatestTag))
}

func exitCodeFor(err error) int {
	if err == nil {
		return 0
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
	return 1
}

// versionCmd prints the same template as the --version flag. Exists so
// `okdctl version` works alongside `okdctl --version` (kubectl/docker/gh
// all expose both).
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, git commit, build date",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("okdctl %s\nGit Commit: %s\nBuild Date: %s\nGo Version: %s\nPlatform:   %s\n",
			version.Version, version.GitCommit, version.BuildDate, version.GoVersion, version.Platform)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "okdctl.yaml", "configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log verbosity (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log output format (text, json)")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "write log output to this file in addition to stderr")
	rootCmd.PersistentFlags().BoolVarP(&logQuiet, "quiet", "q", false, "suppress info/warn logs (alias for --log-level=error)")
	rootCmd.PersistentFlags().BoolVarP(&logVerbose, "verbose", "v", false, "enable debug logging (alias for --log-level=debug)")
	rootCmd.MarkFlagsMutuallyExclusive("quiet", "verbose")

	// EX_USAGE (64, per BSD sysexits.h) for cobra arg-parse failures.
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		tui.Error(err.Error())
		os.Exit(64)
		return err
	})

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
