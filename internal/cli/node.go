package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	nodeYes            bool
	nodeConfirmCluster string
	nodeDryRun         bool
	nodeForceStorage   bool
	nodeSkipDrain      bool
	nodeDrainTimeout   string
	nodeResizeMemoryMB int
	nodeResizeCPU      int
)

var nodeCmd = &cobra.Command{
	Use:     "node",
	Aliases: []string{"nodes"},
	Short:   "Manage cluster node lifecycle",
	Long: `Add, remove, and resize cluster nodes as first-class operations that span
Proxmox VMs (via Terraform), Terraform state, and the Kubernetes lifecycle
(cordon, drain, CSR, etcd-quorum safety).`,
}

var nodeRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a worker node from the cluster",
	Long: `Cordon and drain a worker, destroy its VM via a plan-gated targeted
terraform apply, then delete its Kubernetes Node object.

Only the highest-numbered worker is removable: terraform count reduction
destroys the last instance, so workers must be removed top-down. Guards refuse
removal when the worker holds rook-ceph OSDs (data loss) or when router pods
run on workers with a non-schedulable control plane (ingress outage).`,
	Example: `  okdctl node remove worker2 --yes --confirm-cluster grappleberry
  okdctl node remove worker2 --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runNodeRemove,
}

var nodeResizeCmd = &cobra.Command{
	Use:   "resize (masters|workers|<name>) --memory-mb N [--cpu N]",
	Short: "Resize node CPU/memory per role, rolled out one node at a time",
	Long: `Change per-role node resources and roll the change out one node at a
time. Masters are etcd-health-gated before and after every node and applied
with an in-place-update plan gate (a VM replace is refused). Workers roll
without the etcd gate.

Sizing is per-role: the role's memory/cpu knob is updated in config and tfvars,
but each targeted apply mutates only the current node; other same-role nodes
pick up the pending change on the next full deploy.`,
	Example: `  okdctl node resize masters --memory-mb 24576 --yes --confirm-cluster grappleberry
  okdctl node resize workers --memory-mb 16384 --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runNodeResize,
}

var nodeAddCmd = &cobra.Command{
	Use:   "add --role worker",
	Short: "Add a node to the cluster (not yet implemented)",
	Long: `Adding a node requires building and uploading a per-node CoreOS ISO and
reviving the ignition HTTPS server post-install; it is deferred (see the node
lifecycle spec, phase 4). Use 'okdctl deploy' to grow a fresh cluster.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return &errtypes.UsageError{Msg: "node add is not yet implemented (deferred to phase 4); see 'okdctl node --help'"}
	},
}

func init() {
	nodeRemoveCmd.Flags().BoolVarP(&nodeYes, "yes", "y", false, "skip confirmation prompt")
	nodeRemoveCmd.Flags().StringVar(&nodeConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	nodeRemoveCmd.Flags().BoolVar(&nodeDryRun, flagDryRun, false, "run guards and the plan gate without mutating anything")
	nodeRemoveCmd.Flags().BoolVar(&nodeForceStorage, "force-storage", false, "allow removal even when the worker holds rook-ceph OSDs (destroys their data disk)")
	nodeRemoveCmd.Flags().BoolVar(&nodeSkipDrain, "skip-drain", false, "skip cordon/drain (assumes the node is already evacuated)")
	nodeRemoveCmd.Flags().StringVar(&nodeDrainTimeout, "drain-timeout", "10m", "per-node drain timeout")

	nodeResizeCmd.Flags().BoolVarP(&nodeYes, "yes", "y", false, "skip confirmation prompt")
	nodeResizeCmd.Flags().StringVar(&nodeConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	nodeResizeCmd.Flags().BoolVar(&nodeDryRun, flagDryRun, false, "run gates and the plan gate without mutating anything")
	nodeResizeCmd.Flags().IntVar(&nodeResizeMemoryMB, "memory-mb", 0, "new per-node memory in MiB (required)")
	nodeResizeCmd.Flags().IntVar(&nodeResizeCPU, "cpu", 0, "new per-node cpu cores (0 leaves unchanged)")

	nodeAddCmd.Flags().String("role", "worker", "node role to add")

	nodeCmd.AddCommand(nodeRemoveCmd)
	nodeCmd.AddCommand(nodeResizeCmd)
	nodeCmd.AddCommand(nodeAddCmd)
	rootCmd.AddCommand(nodeCmd)
}

// nodeRunnerCtx bundles the disposable resources a node op holds so RunE
// bodies can defer a single cleanup. HostTotalMiB/HostAllocatedMiB are the
// read-only Proxmox memory-budget probe results (zero when the probe was
// skipped or failed — the guard then warns instead of enforcing).
type nodeRunnerCtx struct {
	runner           *node.Runner
	release          func()
	HostTotalMiB     int
	HostAllocatedMiB int
}

func (n *nodeRunnerCtx) cleanup() { n.release() }

// buildNodeRunner resolves the workspace, migrates the terraform root if it
// predates node-lifecycle support, loads credentials, and wires a node.Runner
// under the project run lock. The returned cleanup zeroizes credentials and
// releases the lock.
func buildNodeRunner(ctx context.Context, cfg *config.Config, verb string, dryRun, probeHost bool) (*nodeRunnerCtx, error) {
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return nil, err
	}
	if err := ensureNodeOpsWorkspace(ctx, projectRoot, nodeYes); err != nil {
		return nil, err
	}

	creds, err := handleCredentials(cfg)
	if err != nil {
		return nil, err
	}

	var hostTotalMiB, hostAllocatedMiB int
	if probeHost && creds.IsValid() {
		hostTotalMiB, hostAllocatedMiB = runHostBudgetProbe(ctx, cfg, creds)
	}

	tfEnv := phase.GetTerraformEnv(cfg)
	terraformDir := filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv)
	tfOpts := []terraform.Option{terraform.WithLogger(tui.SimpleLogger())}
	if creds.IsValid() {
		tfOpts = append(tfOpts, terraform.WithEnv(creds.Env()))
	}
	tf := terraform.New(terraformDir, tfOpts...)

	cl, err := clusterstatus.NewClient(projectRoot)
	if err != nil {
		creds.Zeroize()
		tf.ZeroizeEnv()
		return nil, err
	}

	lock, err := runlock.Acquire(projectRoot, "node "+verb)
	if err != nil {
		creds.Zeroize()
		tf.ZeroizeEnv()
		return nil, err
	}

	runner := node.NewRunner(cl, tf, cfg, projectRoot, cfgFile, tfEnv, tui.RunID(), tui.SimpleLogger())
	runner.DryRun = dryRun
	runner.Reporter = func(desc string) func() { return tui.StartSpinner(ctx, desc) }

	// A resize takes effect only after a Proxmox power-cycle (bpg/proxmox changes
	// the VM config without restarting). Wire the API power-cycler whenever creds
	// are present; resize fails safe when it is nil.
	if probeHost && creds.IsValid() {
		runner.Power = proxmox.NewPowerCycler(&proxmox.PowerCycleOptions{
			Endpoint: creds.Endpoint,
			Username: creds.Username,
			Password: creds.Password,
			APIToken: creds.APIToken,
			Insecure: creds.Insecure,
		})
	}

	return &nodeRunnerCtx{
		runner:           runner,
		HostTotalMiB:     hostTotalMiB,
		HostAllocatedMiB: hostAllocatedMiB,
		release: func() {
			lock.Release()
			tf.ZeroizeEnv()
			creds.Zeroize()
		},
	}, nil
}

// runHostBudgetProbe reads host memory and datastore headroom from the Proxmox
// API (read-only) so the memory-budget guard can enforce numerically. It
// degrades gracefully: a probe failure or missing provider config logs a
// warning and returns zeros, leaving the guard in warn-only mode rather than
// blocking a resize on an unreachable probe.
func runHostBudgetProbe(ctx context.Context, cfg *config.Config, creds *credentials.ProxmoxCredentials) (totalMiB, allocatedMiB int) {
	px := cfg.Provider.Proxmox
	if px == nil {
		return 0, 0
	}
	probe, err := proxmox.ProbeHost(ctx, &proxmox.ProbeOptions{
		Endpoint:   creds.Endpoint,
		Username:   creds.Username,
		Password:   creds.Password,
		APIToken:   creds.APIToken,
		Insecure:   creds.Insecure,
		Node:       px.Node,
		Datastores: []string{px.Storage, px.DataStorage},
	})
	if err != nil {
		tui.Warn("host memory-budget probe failed; memory guard will warn instead of enforce", tui.LF("err", err))
		return 0, 0
	}
	tui.Info("host memory budget",
		tui.LF("node", probe.Node),
		tui.LF("total_mib", probe.HostMemTotalMiB()),
		tui.LF("allocated_mib", probe.GuestAllocatedMiB()))
	for _, d := range probe.Datastores {
		tui.Info("datastore headroom",
			tui.LF("name", d.Name),
			tui.LF("free_gib", d.AvailBytes/(1024*1024*1024)),
			tui.LF("total_gib", d.TotalBytes/(1024*1024*1024)))
	}
	return probe.HostMemTotalMiB(), probe.GuestAllocatedMiB()
}

// ensureNodeOpsWorkspace migrates a write-once terraform root that lacks the
// node-lifecycle variables. Migration overwrites operator-editable HCL, so it
// requires consent (--yes or an interactive y/N) and backs up the originals.
func ensureNodeOpsWorkspace(ctx context.Context, projectRoot string, yes bool) error {
	ok, err := deploy.TerraformRootSupportsNodeOps(projectRoot)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	tui.Warn("terraform root predates node-lifecycle support; it must be migrated (worker_count / master sizing variables)")
	tui.Info("migration backs up the originals to *.pre-nodeops.bak before overwriting")
	if !yes {
		proceed, err := promptForConfirmation(ctx, "migrate the terraform root now? [y/N]: ")
		if err != nil {
			return err
		}
		if !proceed {
			return &errtypes.ConfigError{Msg: "terraform root migration declined; node ops cannot run against the older root"}
		}
	}
	migrated, err := deploy.MigrateTerraformRoot(projectRoot)
	if err != nil {
		return &errtypes.ConfigError{Msg: "migrate terraform root", Err: err}
	}
	for _, f := range migrated {
		tui.Info("migrated terraform file", tui.LF("path", f))
	}
	return nil
}

func runNodeRemove(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	target := args[0]

	if err := confirmClusterMatches(nodeYes, nodeConfirmCluster, cfg.Cluster.Name, "node remove"); err != nil {
		return err
	}
	if !nodeYes && !nodeDryRun {
		proceed, err := promptForConfirmation(cmd.Context(), fmt.Sprintf("remove node %q from cluster %q? [y/N]: ", target, cfg.Cluster.Name))
		if err != nil {
			return err
		}
		if !proceed {
			tui.Info("cancelled")
			return nil
		}
	}

	rc, err := buildNodeRunner(cmd.Context(), cfg, "remove", nodeDryRun, false)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	return rc.runner.RemoveWorker(cmd.Context(), target, node.RemoveOptions{
		ForceStorage: nodeForceStorage,
		SkipDrain:    nodeSkipDrain,
		DrainTimeout: nodeDrainTimeout,
	})
}

func runNodeResize(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	scope, err := parseResizeScope(args[0])
	if err != nil {
		return err
	}
	if nodeResizeMemoryMB <= 0 {
		return &errtypes.UsageError{Msg: "resize requires --memory-mb greater than 0"}
	}

	if err := confirmClusterMatches(nodeYes, nodeConfirmCluster, cfg.Cluster.Name, "node resize"); err != nil {
		return err
	}
	if !nodeYes && !nodeDryRun {
		proceed, err := promptForConfirmation(cmd.Context(), fmt.Sprintf("resize %s to %d MiB in cluster %q? [y/N]: ", args[0], nodeResizeMemoryMB, cfg.Cluster.Name))
		if err != nil {
			return err
		}
		if !proceed {
			tui.Info("cancelled")
			return nil
		}
	}

	rc, err := buildNodeRunner(cmd.Context(), cfg, "resize", nodeDryRun, true)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	return rc.runner.Resize(cmd.Context(), scope, node.ResizeOptions{
		MemoryMB:         nodeResizeMemoryMB,
		CPU:              nodeResizeCPU,
		HostTotalMiB:     rc.HostTotalMiB,
		HostAllocatedMiB: rc.HostAllocatedMiB,
	})
}

func parseResizeScope(arg string) (node.ResizeScope, error) {
	switch arg {
	case "masters", "master":
		return node.ResizeScope{Role: nodetypes.RoleMaster}, nil
	case "workers", "worker":
		return node.ResizeScope{Role: nodetypes.RoleWorker}, nil
	default:
		if _, ok := cluster.NodeIndex(arg); !ok {
			return node.ResizeScope{}, &errtypes.UsageError{Msg: fmt.Sprintf("resize target %q must be 'masters', 'workers', or a node name with a numeric suffix", arg)}
		}
		return node.ResizeScope{Node: arg}, nil
	}
}
