package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/cleanup"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	cleanupYes            bool
	cleanupDryRun         bool
	cleanupConfirmCluster string
	cleanupKind           string
)

var cleanupCmd = &cobra.Command{
	Use:   cmdNameCleanup,
	Short: "Remove OKD cluster artifacts without destroying infrastructure",
	Long: `Remove cluster artifacts (work directory, ignition files, HAProxy,
dnsmasq, Apache httpd, Terraform state files) without tearing down
Proxmox infrastructure.

Use this after a manual Terraform destroy, or to reset a failed deployment
to a clean state.

--kind scopes cleanup to a single subsystem instead of the "full" default.`,
	Example: `  okdctl cleanup
  okdctl cleanup --yes
  okdctl cleanup --kind work-only
  okdctl cleanup --dry-run`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVarP(&cleanupYes, "yes", "y", false, "skip confirmation prompt")
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, flagDryRun, false, "preview what would be removed without making changes")
	cleanupCmd.Flags().StringVar(&cleanupConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal cfg.Cluster.Name (typo guard for scripted cleanups)")
	cleanupCmd.Flags().StringVar(&cleanupKind, "kind", string(cleanup.Full),
		"cleanup scope: "+strings.Join(cleanup.KindStrings(), ", "))
	_ = cleanupCmd.RegisterFlagCompletionFunc("kind", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return cleanup.KindStrings(), cobra.ShellCompDirectiveNoFileComp
	})
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanupDryRun(projectRoot string) {
	workDir := filepath.Join(projectRoot, phase.WorkDirName)
	tui.Info("dry-run: would remove work directory", tui.LF("path", workDir))
	tui.Info("dry-run: would remove haproxy config block", tui.LF("path", phase.DefaultHAProxyConfigPath))
	tui.Info("dry-run: would remove dnsmasq drop-in", tui.LF("dir", phase.DefaultDNSMasqConfigDir))
	tui.Info("dry-run: would remove packages", tui.LF("packages", cleanup.InstalledPackages()))
	tui.Info("dry-run: would remove binaries", tui.LF("binaries", cleanup.InstalledBinaries()))
	tui.Info("dry-run: re-run without --dry-run to execute cleanup")
}

// cleanupKindRemovesCredentials reports whether kind wipes cluster-config
// and with it the admin credentials (kubeconfig, kubeadmin-password).
func cleanupKindRemovesCredentials(kind cleanup.Kind) bool {
	return kind == cleanup.Full || kind == cleanup.WorkOnly
}

// confirmCleanupInteractive gates a cleanup run: kinds that remove the
// admin credentials get the same two-stage typed-cluster-name gate as
// destroy; scoped kinds keep the single y/N prompt.
func confirmCleanupInteractive(ctx context.Context, cfg *config.Config, kind cleanup.Kind) (bool, error) {
	if cleanupKindRemovesCredentials(kind) {
		nameConfirmed, err := promptForClusterNameConfirmation(ctx, cfg.Cluster.Name, "type cluster name to confirm cleanup: ")
		if err != nil || !nameConfirmed {
			return false, err
		}
	}
	return promptForConfirmation(ctx, "proceed with cleanup? [y/N]: ")
}

func runCleanup(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	kind := cleanup.Kind(cleanupKind)
	if kind.Validate() != nil {
		// Msg-only UsageError: wrapping the ConfigError from Validate would
		// let exitCodeFor's Config-first precedence map this to exit 2.
		return &errtypes.UsageError{
			Msg: fmt.Sprintf("invalid --kind %q; valid values: %s", cleanupKind, strings.Join(cleanup.KindStrings(), ", ")),
		}
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
	if cleanupKindRemovesCredentials(kind) {
		tui.Warn("once the infrastructure is destroyed this includes the admin credentials (kubeconfig, kubeadmin-password)")
	}

	if err := confirmClusterMatches(cleanupYes, cleanupConfirmCluster, cfg.Cluster.Name, "cleanup"); err != nil {
		return err
	}

	if !cleanupYes {
		confirmed, err := confirmCleanupInteractive(ctx, cfg, kind)
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

	workDir := filepath.Join(projectRoot, phase.WorkDirName)
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			tui.Warn("workdir chown back to user incomplete", tui.LF("err", chownErr))
		}
	}()

	vip, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return err
	}

	opts := cleanup.NewOptions(cfg, projectRoot, kind)
	opts.VIP = vip

	tui.Info("cleaning up cluster artifacts...")
	startTime := time.Now()

	p := deploy.NewProvisioner(nil, projectRoot)
	defer p.ZeroizeEnv()
	if err := p.Cleanup(ctx, &opts); err != nil {
		tui.Warn("partial cleanup; rerun to retry")
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	tui.Info("cleanup complete", tui.LF("duration", duration))

	return nil
}
