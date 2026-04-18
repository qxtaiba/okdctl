package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var cleanupYes bool

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove OKD cluster artifacts without destroying infrastructure",
	Long: `Remove cluster artifacts (work directory, ignition files, HAProxy,
dnsmasq, Apache httpd, Terraform state files) without tearing down
Proxmox infrastructure.

Use this after a manual Terraform destroy, or to reset a failed deployment
to a clean state.`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVarP(&cleanupYes, "yes", "y", false, "skip confirmation prompt")
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	tui.Warn(fmt.Sprintf("this will remove all local artifacts for cluster '%s'", cfg.Cluster.Name))

	if !cleanupYes {
		confirmed, err := promptForConfirmation(ctx, "proceed with cleanup? [y/N]: ")
		if err != nil {
			return err
		}
		if !confirmed {
			tui.Info("cancelled")
			return nil
		}
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	workDir := filepath.Join(projectRoot, "okd-install")
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			tui.Warn(fmt.Sprintf("workdir chown back to user incomplete: %v", chownErr))
		}
	}()

	vip, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return err
	}

	opts := &cleanup.Options{
		BaseOptions: phase.BaseOptions{
			WorkDir:      workDir,
			ProjectRoot:  projectRoot,
			TerraformEnv: phase.GetTerraformEnv(cfg),
		},
		Kind:           cleanup.Full,
		HTTPServerRoot: cfg.HTTPServer.Root,
		HAProxyConfig:  phase.DefaultHAProxyConfigPath,
		VIP:            vip,
		ClusterName:    cfg.Cluster.Name,
		PreserveConfig: false,
		RemovePackages: false,
		Logger:         tui.SimpleLogger(),
	}

	tui.Info("cleaning up cluster artifacts...")
	startTime := time.Now()

	if err := cleanup.Execute(ctx, opts); err != nil {
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info(fmt.Sprintf("cleanup complete (%s)", duration))

	return nil
}
