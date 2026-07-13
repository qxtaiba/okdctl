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

func init() {
	clusterCompactCmd.Flags().BoolVarP(&compactYes, "yes", "y", false, "skip confirmation prompt")
	clusterCompactCmd.Flags().StringVar(&compactConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	clusterCompactCmd.Flags().BoolVar(&compactDryRun, flagDryRun, false, "print the compaction plan without mutating anything")
	clusterCompactCmd.Flags().BoolVar(&compactForceStorage, "force-storage", false, "allow worker removal even when workers hold rook-ceph OSDs")
	clusterCompactCmd.Flags().IntVar(&compactIngressReplica, "ingress-replicas", 2, "compact IngressController replica count")
	clusterCompactCmd.Flags().IntVar(&compactGrowMasterMB, "grow-master-memory-mb", 0, "resize each master to this memory (MiB) as workers are removed (0 leaves masters unchanged)")

	clusterCmd.AddCommand(clusterCompactCmd)
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

	consent := nodeConsent{yes: compactYes, confirmCluster: compactConfirmCluster, dryRun: compactDryRun, twoStage: true}
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
