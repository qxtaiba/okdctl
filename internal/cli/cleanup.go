package cli

import (
	"context"
	"fmt"
	"io"
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
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// cleanupIrreversibleWarning is the ConfirmBox irreversible text for a
// cleanup kind that wipes admin credentials (kubeconfig, kubeadmin-password).
const cleanupIrreversibleWarning = "wipes cluster credentials (kubeconfig, kubeadmin-password); they cannot be recovered without a fresh deploy"

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

// runCleanupDryRun mirrors cleanup.cleanupSteps' switch so the preview cannot drift from execution.
func runCleanupDryRun(w io.Writer, cfg *config.Config, projectRoot string, kind cleanup.Kind) {
	workDir := "remove work directory (" + workspace.WorkDir(projectRoot) + ")"
	webServer := "remove ignition files from web server (" + cfg.HTTPServer.Root + ")"
	haproxy := "stop haproxy and remove its config block (" + phase.DefaultHAProxyConfigPath + ")"
	apache := "stop apache httpd service"
	dnsmasq := "stop dnsmasq and remove its drop-in (" + phase.DefaultDNSMasqConfigDir + ")"
	terraformArtifacts := "remove generated terraform artifacts and the post-destroy tfstate (env=" + cfg.TerraformEnvName() + ")"
	packages := "remove packages (" + strings.Join(cleanup.InstalledPackages(), ", ") + ") and tool binaries (" + strings.Join(cleanup.InstalledBinaries(), ", ") + ")"
	ignitionCerts := "remove generated ignition TLS certs (" + filepath.Join(projectRoot, "certs", "ignition") + ")"

	var would []string
	switch kind {
	case cleanup.Full:
		would = []string{workDir, webServer, haproxy, apache, dnsmasq, terraformArtifacts, packages, ignitionCerts}
	case cleanup.WorkOnly:
		would = []string{workDir}
	case cleanup.WebOnly:
		would = []string{webServer}
	case cleanup.HAProxyOnly:
		would = []string{haproxy}
	case cleanup.TerraformOnly:
		would = []string{terraformArtifacts}
	}

	fmt.Fprintln(w, render.DryRunActions("cleanup", cleanupConfirmFacts(cfg, projectRoot, kind), would))
}

// cleanupKindRemovesCredentials reports whether kind wipes kubeconfig/kubeadmin-password.
func cleanupKindRemovesCredentials(kind cleanup.Kind) bool {
	return kind == cleanup.Full || kind == cleanup.WorkOnly
}

// confirmCleanupInteractive requires typed-name confirmation for
// credential-removing kinds; scoped kinds keep a single y/N.
func confirmCleanupInteractive(ctx context.Context, cfg *config.Config, kind cleanup.Kind) (bool, error) {
	if cleanupKindRemovesCredentials(kind) {
		nameConfirmed, err := promptForClusterNameConfirmation(ctx, cfg.Cluster.Name, tui.PromptLine("type cluster name to confirm cleanup"))
		if err != nil || !nameConfirmed {
			return false, err
		}
	}
	return promptForConfirmation(ctx, tui.PromptLine("proceed with cleanup? [y/N]"))
}

// cleanupConfirmFacts builds the ConfirmBox facts for cfg's cluster, kind,
// and the resolved work directory.
func cleanupConfirmFacts(cfg *config.Config, projectRoot string, kind cleanup.Kind) []render.Fact {
	return []render.Fact{
		{Key: factKeyCluster, Value: cfg.Cluster.Name},
		{Key: "kind", Value: string(kind)},
		{Key: "work dir", Value: workspace.WorkDir(projectRoot)},
	}
}

func runCleanup(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	kind := cleanup.Kind(cleanupKind)
	if kind.Validate() != nil {
		// Msg-only: wrapping Validate's ConfigError would map to exit 2 via exitCodeFor's precedence.
		return &errtypes.UsageError{
			Msg: fmt.Sprintf("invalid --kind %q; valid values: %s", cleanupKind, strings.Join(cleanup.KindStrings(), ", ")),
		}
	}

	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	if cleanupDryRun {
		runCleanupDryRun(cmd.OutOrStdout(), cfg, projectRoot, kind)
		return nil
	}

	irreversible := ""
	if cleanupKindRemovesCredentials(kind) {
		irreversible = cleanupIrreversibleWarning
	}
	fmt.Fprintln(cmd.ErrOrStderr(), render.ConfirmBox("cleanup", cleanupConfirmFacts(cfg, projectRoot, kind), irreversible))

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
