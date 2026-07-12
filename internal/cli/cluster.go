package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui"
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
	if !compactYes && !compactDryRun {
		proceed, err := promptForConfirmation(cmd.Context(), fmt.Sprintf("compact cluster %q onto its control plane? [y/N]: ", cfg.Cluster.Name))
		if err != nil {
			return err
		}
		if !proceed {
			tui.Info("cancelled")
			return nil
		}
	}

	// buildNodeRunner reads the shared node* flag vars; align them with the
	// compact flag set so migration consent and dry-run route correctly.
	nodeYes = compactYes
	rc, err := buildNodeRunner(cmd.Context(), cfg, "compact", compactDryRun)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	return rc.runner.Compact(cmd.Context(), node.CompactOptions{
		IngressReplicas:    compactIngressReplica,
		GrowMasterMemoryMB: compactGrowMasterMB,
		ForceStorage:       compactForceStorage,
	})
}
