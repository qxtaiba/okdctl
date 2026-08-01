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
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
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
	Args: cobra.NoArgs,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVarP(&cleanupYes, "yes", "y", false, "skip confirmation prompt")
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, flagDryRun, false, "preview what would be removed without making changes")
	cleanupCmd.Flags().StringVar(&cleanupConfirmCluster, "confirm-cluster", "",
		"required with --yes; must equal the config cluster name")
	cleanupCmd.Flags().StringVar(&cleanupKind, "kind", string(cleanup.Full),
		"cleanup scope: "+strings.Join(cleanup.KindStrings(), ", "))
	_ = cleanupCmd.RegisterFlagCompletionFunc("kind", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return cleanup.KindStrings(), cobra.ShellCompDirectiveNoFileComp
	})
	rootCmd.AddCommand(cleanupCmd)
}

type cleanupDryRunTarget struct {
	msg    string
	fields []logutil.LogField
}

// runCleanupDryRun previews exactly the targets the given kind removes. The
// per-kind selection mirrors cleanup.cleanupSteps' switch so the preview
// cannot drift from execution — the cleanup Phase deliberately exposes no
// StepDefs to the CLI (see cleanup.Phase), so the switch is reproduced here.
func runCleanupDryRun(cfg *config.Config, projectRoot string, kind cleanup.Kind) {
	for _, t := range cleanupDryRunTargets(cfg, projectRoot, kind) {
		logutil.Info(t.msg, t.fields...)
	}
	logutil.Info("dry-run: re-run without --dry-run to execute cleanup")
}

func cleanupDryRunTargets(cfg *config.Config, projectRoot string, kind cleanup.Kind) []cleanupDryRunTarget {
	workDir := cleanupDryRunTarget{"dry-run: would remove work directory", []logutil.LogField{logutil.LF("path", workspace.WorkDir(projectRoot))}}
	webServer := cleanupDryRunTarget{"dry-run: would remove ignition files from web server", []logutil.LogField{logutil.LF("dir", cfg.HTTPServer.Root)}}
	haproxy := cleanupDryRunTarget{"dry-run: would stop haproxy and remove its config block", []logutil.LogField{logutil.LF("path", phase.DefaultHAProxyConfigPath)}}
	apache := cleanupDryRunTarget{msg: "dry-run: would stop apache httpd service"}
	dnsmasq := cleanupDryRunTarget{"dry-run: would stop dnsmasq and remove its drop-in", []logutil.LogField{logutil.LF("dir", phase.DefaultDNSMasqConfigDir)}}
	terraform := cleanupDryRunTarget{"dry-run: would remove generated terraform artifacts and the post-destroy tfstate", []logutil.LogField{logutil.LF("env", cfg.TerraformEnvName())}}
	packages := cleanupDryRunTarget{"dry-run: would remove packages and tool binaries", []logutil.LogField{logutil.LF("packages", cleanup.InstalledPackages()), logutil.LF("binaries", cleanup.InstalledBinaries())}}
	ignitionCerts := cleanupDryRunTarget{"dry-run: would remove generated ignition TLS certs", []logutil.LogField{logutil.LF("path", filepath.Join(projectRoot, "certs", "ignition"))}}

	switch kind {
	case cleanup.Full:
		return []cleanupDryRunTarget{workDir, webServer, haproxy, apache, dnsmasq, terraform, packages, ignitionCerts}
	case cleanup.WorkOnly:
		return []cleanupDryRunTarget{workDir}
	case cleanup.WebOnly:
		return []cleanupDryRunTarget{webServer}
	case cleanup.HAProxyOnly:
		return []cleanupDryRunTarget{haproxy}
	case cleanup.TerraformOnly:
		return []cleanupDryRunTarget{terraform}
	default:
		return nil
	}
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
		runCleanupDryRun(cfg, projectRoot, kind)
		return nil
	}

	logutil.Warn("this will remove all local artifacts for cluster", logutil.LF("cluster", cfg.Cluster.Name))
	if cleanupKindRemovesCredentials(kind) {
		logutil.Warn("once the infrastructure is destroyed this includes the admin credentials (kubeconfig, kubeadmin-password)")
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
			logutil.Info("cancelled")
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

	workDir := workspace.WorkDir(projectRoot)
	defer func() {
		if chownErr := system.ChownTreeToInvokingUser(workDir); chownErr != nil {
			logutil.Warn("workdir chown back to user incomplete", logutil.LF("err", chownErr))
		}
	}()

	vip, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return err
	}

	opts := cleanup.NewOptions(cfg, projectRoot, kind)
	opts.VIP = vip

	logutil.Info("cleaning up cluster artifacts...")
	startTime := time.Now()

	p := deploy.NewProvisioner(nil, projectRoot)
	defer p.ZeroizeEnv()
	if err := p.Cleanup(ctx, &opts); err != nil {
		logutil.Warn("partial cleanup; rerun to retry")
		return err
	}

	duration := time.Since(startTime).Round(time.Second)
	logutil.Info("cleanup complete", logutil.LF("duration", duration))

	return nil
}
