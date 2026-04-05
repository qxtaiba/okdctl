package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/qxtaiba/okd-proxmox-cli/internal/deployment"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/pkg/version"
)

// cfgFile is package-scope state managed by cobra PersistentFlags. It is
// populated once by the root command's --config flag and read by subcommand
// RunE handlers (deploy, destroy, update-ingress) via direct package
// reference. This is the standard cobra pattern; threading it through
// function parameters would fight the framework.
var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "openshitctl",
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
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(tui.TitleStyle.Render("Homelab K8s"))
		fmt.Println()
		fmt.Println(tui.MutedStyle.Render("Quick start:"))
		fmt.Println("  " + tui.HighlightStyle.Render("openshitctl deploy") + "           Deploy a cluster")
		fmt.Println("  " + tui.HighlightStyle.Render("openshitctl destroy") + "          Destroy the cluster")
		fmt.Println("  " + tui.HighlightStyle.Render("openshitctl update-ingress") + "   Switch ingress to LoadBalancer IPs")
		fmt.Println()
		fmt.Println(tui.MutedStyle.Render("Run 'openshitctl --help' for all commands"))
	},
}

func Execute() {
	slog.SetDefault(tui.SimpleLogger())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, deployment.ErrInterrupted) {
			os.Exit(130)
		}
		tui.Error(err.Error())
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "openshitctl.yaml", "configuration file")

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

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("openshitctl")
		viper.SetConfigType("yaml")

		viper.AddConfigPath(".")
		if home, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(home + "/.openshitctl")
		}

		viper.AddConfigPath("/etc/openshitctl")
	}

	viper.SetEnvPrefix("HOMELAB")
	viper.AutomaticEnv()

	_ = viper.ReadInConfig()
}

