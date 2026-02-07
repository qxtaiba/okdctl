package cli

import (
	"github.com/spf13/cobra"

	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/pkg/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print detailed version information about the openshitctl CLI",
	Run:   runVersion,
}

func runVersion(cmd *cobra.Command, args []string) {
	info := version.Get()

	tui.Info("openshitctl cli")
	tui.Info("version: " + info.Version)
	tui.Info("git commit: " + info.GitCommit)
	tui.Info("build date: " + info.BuildDate)
	tui.Info("go version: " + info.GoVersion)
	tui.Info("platform: " + info.Platform)
}
