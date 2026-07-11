package cli

import (
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	cleanupYes            bool
	cleanupDryRun         bool
	cleanupConfirmCluster string
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove OKD cluster artifacts without destroying infrastructure",
	Long: `Remove cluster artifacts (work directory, ignition files, HAProxy,
dnsmasq, Apache httpd, Terraform state files) without tearing down
Proxmox infrastructure.

Use this after a manual Terraform destroy, or to reset a failed deployment
to a clean state.`,
	Example: `  okdctl cleanup
  okdctl cleanup --yes
  okdctl cleanup --dry-run`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVarP(&cleanupYes, "yes", "y", false, "skip confirmation prompt")
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, flagDryRun, false, "preview what would be removed without making changes")
	cleanupCmd.Flags().StringVar(&cleanupConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal cfg.Cluster.Name (typo guard for scripted cleanups)")
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanupDryRun(projectRoot string) {
	workDir := filepath.Join(projectRoot, "okd-install")
	tui.Info("dry-run: would remove work directory", tui.LF("path", workDir))
	tui.Info("dry-run: would remove haproxy config block", tui.LF("path", phase.DefaultHAProxyConfigPath))
	tui.Info("dry-run: would remove dnsmasq drop-in", tui.LF("dir", phase.DefaultDNSMasqConfigDir))
	tui.Info("dry-run: would remove packages", tui.LF("packages", cleanup.InstalledPackages()))
	tui.Info("dry-run: would remove binaries", tui.LF("binaries", cleanup.InstalledBinaries()))
	tui.Info("dry-run: re-run without --dry-run to execute cleanup")
}

func runCleanup(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if cleanupDryRun {
		projectRoot, err := resolveProjectRootOrDie()
		if err != nil {
			return err
		}
		runCleanupDryRun(projectRoot)
		return nil
	}

	tui.Warn("this will remove all local artifacts for cluster", tui.LF("cluster", cfg.Cluster.Name))

	if err := confirmClusterMatches(cleanupYes, cleanupConfirmCluster, cfg.Cluster.Name, "cleanup"); err != nil {
		return err
	}

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

	lock, err := runlock.Acquire(projectRoot, "cleanup")
	if err != nil {
		return err
	}
	defer lock.Release()

	workDir := filepath.Join(projectRoot, "okd-install")
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			tui.Warn("workdir chown back to user incomplete", tui.LF("err", chownErr))
		}
	}()

	vip, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return err
	}

	logger := tui.SimpleLogger()
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
		BinDir:         config.ResolveBinDir(cfg),
	}

	tui.Info("cleaning up cluster artifacts...")
	startTime := time.Now()

	exec := executor.New(executor.WithWorkDir(projectRoot))
	if err := cleanup.New(phase.WithExecutor(exec), phase.WithLogger(logger)).Execute(ctx, opts); err != nil {
		tui.Warn("partial cleanup; rerun to retry")
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info("cleanup complete", tui.LF("duration", duration))

	return nil
}
