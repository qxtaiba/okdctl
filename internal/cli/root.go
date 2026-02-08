// Package cli implements the openshitctl command-line interface.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/qxtaiba/okd-proxmox-cli/internal/deployment"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/pkg/version"
)

var (
	cfgFile string
	verbose bool
	noColor bool
)

var rootCmd = &cobra.Command{
	Use:   "openshitctl",
	Short: "Deploy production-ready Kubernetes clusters",
	Long: `Homelab K8s - Deploy production-ready Kubernetes clusters

A delightful CLI tool for deploying OKD/OpenShift clusters
on Proxmox VE infrastructure.

Highlights:
  • Interactive setup wizard with beautiful TUI
  • OKD/OpenShift 4.15-4.21 support
  • Addon-extensible architecture (MetalLB, Ingress, Flux, storage, cert-manager)
  • YAML configuration with sensible defaults
  • Automated preflight checks and validation
  • Single binary distribution`,
	Version: version.Version,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(tui.TitleStyle.Render("Homelab K8s"))
		fmt.Println()
		fmt.Println(tui.MutedStyle.Render("Quick start:"))
		fmt.Println("  " + tui.HighlightStyle.Render("openshitctl deploy") + "    Deploy a cluster")
		fmt.Println("  " + tui.HighlightStyle.Render("openshitctl destroy") + "   Destroy the cluster")
		fmt.Println()
		fmt.Println(tui.MutedStyle.Render("Run 'openshitctl --help' for all commands"))
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
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
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

	if err := viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		tui.Warn("failed to bind verbose flag: " + err.Error())
	}
	if err := viper.BindPFlag("no-color", rootCmd.PersistentFlags().Lookup("no-color")); err != nil {
		tui.Warn("failed to bind no-color flag: " + err.Error())
	}

	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(addonCmd)
	rootCmd.AddCommand(versionCmd)

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

	if err := viper.ReadInConfig(); err == nil {
		if verbose {
			tui.Info("using config file: " + viper.ConfigFileUsed())
		}
	}
}

