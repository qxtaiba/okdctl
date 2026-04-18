package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/version"
)

// cfgFile is package-scope state managed by cobra PersistentFlags. It is
// populated once by the root command's --config flag and read by subcommand
// RunE handlers (deploy, destroy, update-ingress) via direct package
// reference. This is the standard cobra pattern; threading it through
// function parameters would fight the framework.
var cfgFile string

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
	Version:           version.Version,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error { return ensureRoot(cmd) },
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
	os.Exit(execute())
}

func execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if ctx.Err() != nil {
			return 130
		}
		tui.Error(err.Error())
		return 1
	}
	return 0
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "okdctl.yaml", "configuration file")

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
