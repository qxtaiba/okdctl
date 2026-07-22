package cli

import (
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/node"
)

var (
	compactYes            bool
	compactConfirmCluster string
	compactDryRun         bool
	compactForceStorage   bool
	compactIngressReplica int
	compactGrowMasterMB   int

	stopYes            bool
	stopConfirmCluster string
	stopDryRun         bool

	startYes            bool
	startConfirmCluster string
	startDryRun         bool
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Cluster-wide lifecycle operations",
	Long:  "Orchestrate multi-node lifecycle sequences such as compaction onto the control plane.",
}

var clusterCompactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Consolidate the cluster onto its control plane",
	Long: `Make the control plane schedulable, apply the compact IngressController,
then remove workers top-down — optionally growing masters interleaved so a
freed worker always precedes a grown master (memory-budget ordering).

This is a thin orchestrator over 'node remove' and 'node resize'; it adds no
new mutation mechanics and inherits their guards (storage, ingress, etcd).`,
	Example: `  okdctl cluster compact --yes --confirm-cluster grappleberry
  okdctl cluster compact --grow-master-memory-mb 24576 --dry-run`,
	RunE: runClusterCompact,
}

var clusterStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Power off the cluster",
	Long: `Cordon every node, then gracefully power off each worker (ascending)
followed by each master (ascending) through the Proxmox API.

Stop runs no drain: with the whole cluster stopping there is nowhere left to
reschedule a pod. The kubelet client-cert signer's remaining validity is
reported before the confirmation prompt, since it keeps expiring while the
cluster is stopped. Restart with 'okdctl cluster start'.`,
	Example: `  okdctl cluster stop --yes --confirm-cluster grappleberry
  okdctl cluster stop --dry-run`,
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
yet.`,
	Example: `  okdctl cluster start --yes --confirm-cluster grappleberry
  okdctl cluster start --dry-run`,
	RunE: runClusterStart,
}

func init() {
	clusterCompactCmd.Flags().BoolVarP(&compactYes, "yes", "y", false, "skip confirmation prompt")
	clusterCompactCmd.Flags().StringVar(&compactConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	clusterCompactCmd.Flags().BoolVar(&compactDryRun, flagDryRun, false, "print the compaction plan without mutating anything")
	clusterCompactCmd.Flags().BoolVar(&compactForceStorage, "force-storage", false, "allow worker removal even when workers hold rook-ceph OSDs")
	clusterCompactCmd.Flags().IntVar(&compactIngressReplica, "ingress-replicas", 2, "compact IngressController replica count")
	clusterCompactCmd.Flags().IntVar(&compactGrowMasterMB, "grow-master-memory-mb", 0, "resize each master to this memory (MiB) as workers are removed (0 leaves masters unchanged)")

	clusterStopCmd.Flags().BoolVarP(&stopYes, "yes", "y", false, "skip confirmation prompt")
	clusterStopCmd.Flags().StringVar(&stopConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	clusterStopCmd.Flags().BoolVar(&stopDryRun, flagDryRun, false, "print the shutdown plan without powering anything off")

	clusterStartCmd.Flags().BoolVarP(&startYes, "yes", "y", false, "skip confirmation prompt")
	clusterStartCmd.Flags().StringVar(&startConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	clusterStartCmd.Flags().BoolVar(&startDryRun, flagDryRun, false, "print the power-on plan without powering anything on")

	clusterCmd.AddCommand(clusterCompactCmd)
	clusterCmd.AddCommand(clusterStopCmd)
	clusterCmd.AddCommand(clusterStartCmd)
	rootCmd.AddCommand(clusterCmd)
}

func runClusterCompact(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if err := confirmClusterMatches(compactYes, compactConfirmCluster, cfg.Cluster.Name, "cluster compact"); err != nil {
		return err
	}

	consent := nodeConsent{yes: compactYes, dryRun: compactDryRun, twoStage: true}
	rc, err := buildNodeRunner(cmd.Context(), cfg, "compact", consent, true)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	start := time.Now()
	if err := rc.runner.Compact(cmd.Context(), node.CompactOptions{
		IngressReplicas:    compactIngressReplica,
		GrowMasterMemoryMB: compactGrowMasterMB,
		ForceStorage:       compactForceStorage,
		HostTotalMiB:       rc.HostTotalMiB,
		HostAllocatedMiB:   rc.HostAllocatedMiB,
	}); err != nil {
		if errors.Is(err, node.ErrDeclined) {
			return nil
		}
		return err
	}
	rc.complete(cmd.OutOrStdout(), time.Since(start))
	return nil
}

func runClusterStop(cmd *cobra.Command, _ []string) error {
	return runClusterPower(cmd, "stop", stopYes, stopConfirmCluster, stopDryRun,
		func(rc *nodeRunnerCtx) error { return rc.runner.Stop(cmd.Context()) })
}

func runClusterStart(cmd *cobra.Command, _ []string) error {
	return runClusterPower(cmd, "start", startYes, startConfirmCluster, startDryRun,
		func(rc *nodeRunnerCtx) error { return rc.runner.Start(cmd.Context()) })
}

// runClusterPower is the shared shape behind cluster stop and cluster start:
// both are single-stage (twoStage:false — neither op destroys a VM) whole-
// cluster power ops that differ only in which Runner method they call.
func runClusterPower(cmd *cobra.Command, verb string, yes bool, confirmCluster string, dryRun bool, op func(*nodeRunnerCtx) error) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	if err := confirmClusterMatches(yes, confirmCluster, cfg.Cluster.Name, "cluster "+verb); err != nil {
		return err
	}

	consent := nodeConsent{yes: yes, dryRun: dryRun, twoStage: false}
	rc, err := buildNodeRunner(cmd.Context(), cfg, verb, consent, true)
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
