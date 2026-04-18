// Package cli wires together the cobra command tree and drives the
// top-level event loop. Process exit codes follow a documented contract:
// config error=2, network error=3, cluster error=4, auth error=5,
// other error=1, SIGINT/SIGTERM=130, success=0.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	logLevel  string
	logFormat string
	logFile   string
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
	Version: version.Version,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if err := configureLogging(); err != nil {
			return err
		}
		return ensureRoot(cmd)
	},
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println(tui.TitleStyle.Render("homelab k8s"))
		fmt.Println()
		fmt.Println(tui.MutedStyle.Render("quick start:"))
		fmt.Println("  " + tui.HighlightStyle.Render("okdctl deploy") + "           deploy a cluster")
		fmt.Println("  " + tui.HighlightStyle.Render("okdctl destroy") + "          destroy the cluster")
		fmt.Println("  " + tui.HighlightStyle.Render("okdctl update-ingress") + "   switch ingress to loadbalancer ips")
		fmt.Println()
		fmt.Println(tui.MutedStyle.Render("run 'okdctl --help' for all commands"))
	},
}

func Execute() {
	slog.SetDefault(tui.SimpleLogger())
	code := execute()
	if logFileCloser != nil {
		_ = logFileCloser.Close()
	}
	os.Exit(code)
}

func execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	updateCh := version.BackgroundCheck(ctx)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if ctx.Err() != nil {
			return 130
		}
		tui.Error(err.Error())
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

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "okdctl.yaml", "configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log verbosity (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log output format (text, json)")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "write log output to this file in addition to stdout")

	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(updateIngressCmd)

	rootCmd.SetVersionTemplate(fmt.Sprintf(`{{with .Name}}{{printf "%%s " .}}{{end}}{{printf "%%s" .Version}}
Git Commit: %s
Build Date: %s
Go Version: %s
Platform:   %s
`, version.GitCommit, version.BuildDate, version.GoVersion, version.Platform))
}
