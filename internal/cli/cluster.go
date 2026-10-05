package cli

import (
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/node"
)

var (
	stopYes                    bool
	stopConfirmCluster         string
	stopDryRun                 bool
	stopAcknowledgeInterrupted bool

	startYes                    bool
	startConfirmCluster         string
	startDryRun                 bool
	startAcknowledgeInterrupted bool
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage cluster-wide lifecycle operations",
	Long:  "Power the whole cluster off and back on as one ordered, guarded sequence.",
}

var clusterStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Power off the cluster",
	Long: `Cordon every node, then gracefully power off each worker (ascending)
followed by each master (ascending) through the Proxmox API.

Stop runs no drain: with the whole cluster stopping there is nowhere left to
reschedule a pod. The kubelet client-cert signer's remaining validity is
reported before the confirmation prompt, since it keeps expiring while the
cluster is stopped. Restart with 'okdctl cluster start'.

Stop refuses to run while a marker from any other in-flight node op is
recorded, since stop is not resumable and would otherwise overwrite that op's
resume trail. --acknowledge-interrupted-op overrides the marker and proceeds.

With ha_enabled set, masters are also managed by the Proxmox HA manager,
which may counteract an out-of-band shutdown (its request-state still says
started). Stop warns and proceeds; verify the power state afterwards, or set
the HA request-state to stopped via pvesh first.`,
	Example: `  okdctl cluster stop --yes --confirm-cluster grappleberry
  okdctl cluster stop --dry-run`,
	Args: cobra.NoArgs,
	RunE: runClusterStop,
}

var clusterStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Power on the cluster",
	Long: `Power on every master as one batch, then every worker, then wait for
every node to report Ready — approving pending kubelet CSRs on each poll so a
cluster restarted after certificate rotation rejoins unattended — and finally
uncordon every node.

Node enumeration is config-driven (cfg.Topology counts) rather than the
Kubernetes API: the API is hosted by the very VMs start has not powered on
yet.

Start refuses to run while a marker from any other in-flight node op is
recorded, since start is not resumable and would otherwise overwrite that
op's resume trail. --acknowledge-interrupted-op overrides the marker and
proceeds.`,
	Example: `  okdctl cluster start --yes --confirm-cluster grappleberry
  okdctl cluster start --dry-run`,
	Args: cobra.NoArgs,
	RunE: runClusterStart,
}

func init() {
	clusterStopCmd.Flags().BoolVarP(&stopYes, "yes", "y", false, "skip confirmation prompt")
	clusterStopCmd.Flags().StringVar(&stopConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	clusterStopCmd.Flags().BoolVar(&stopDryRun, flagDryRun, false, "print the shutdown plan without powering anything off")
	clusterStopCmd.Flags().BoolVar(&stopAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "override a stranded marker left by an unrelated op and proceed fresh")

	clusterStartCmd.Flags().BoolVarP(&startYes, "yes", "y", false, "skip confirmation prompt")
	clusterStartCmd.Flags().StringVar(&startConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	clusterStartCmd.Flags().BoolVar(&startDryRun, flagDryRun, false, "print the power-on plan without powering anything on")
	clusterStartCmd.Flags().BoolVar(&startAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "override a stranded marker left by an unrelated op and proceed fresh")

	clusterCmd.AddCommand(clusterStopCmd)
	clusterCmd.AddCommand(clusterStartCmd)
	rootCmd.AddCommand(clusterCmd)
}

func runClusterStop(cmd *cobra.Command, _ []string) error {
	return runClusterPower(cmd, "stop", stopYes, stopConfirmCluster, stopDryRun,
		func(rc *nodeRunnerCtx) error {
			return rc.runner.Stop(cmd.Context(), node.StopOptions{Acknowledge: stopAcknowledgeInterrupted})
		})
}

func runClusterStart(cmd *cobra.Command, _ []string) error {
	return runClusterPower(cmd, "start", startYes, startConfirmCluster, startDryRun,
		func(rc *nodeRunnerCtx) error {
			return rc.runner.Start(cmd.Context(), node.StartOptions{Acknowledge: startAcknowledgeInterrupted})
		})
}

// runClusterPower is shared by stop/start; both stay off the destroy-grade gate
// since neither destroys a VM.
func runClusterPower(cmd *cobra.Command, verb string, yes bool, confirmCluster string, dryRun bool, op func(*nodeRunnerCtx) error) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if err := confirmClusterMatches(yes, confirmCluster, cfg.Cluster.Name, "cluster "+verb); err != nil {
		return err
	}

	consent := nodeConsent{yes: yes, dryRun: dryRun, twoStage: destroyGradeVerbs[verb]}
	rc, err := buildNodeRunner(cmd, cfg, verb, consent, true)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	start := time.Now()
	if err := op(rc); err != nil {
		if errors.Is(err, node.ErrDeclined) {
			return nil
		}
		return err
	}
	rc.complete(cmd.OutOrStdout(), time.Since(start))
	return nil
}
