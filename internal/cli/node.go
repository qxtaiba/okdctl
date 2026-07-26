package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/deploy"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
)

var (
	nodeYes                    bool
	nodeConfirmCluster         string
	nodeDryRun                 bool
	nodeForceStorage           bool
	nodeSkipDrain              bool
	nodeDrainTimeout           string
	nodeResizeMemoryMB         int
	nodeResizeCPU              int
	nodeAcknowledgeInterrupted bool
	nodeAddRole                string
	nodeAddCount               int
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
run on workers with a non-schedulable control plane (ingress outage).

An interrupted removal records an op marker and resumes automatically on the
next 'okdctl node remove' of the same worker, skipping already-completed
steps. --acknowledge-interrupted-op overrides a marker left by a different op
or node instead of refusing.`,
	Example: `  okdctl node remove worker2 --yes --confirm-cluster grappleberry
  okdctl node remove worker2 --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runNodeRemove,
}

var nodeResizeCmd = &cobra.Command{
	Use:   "resize (masters|workers|<name>) [--memory-mb N] [--cpu N]",
	Short: "Resize node CPU/memory per role, rolled out one node at a time",
	Long: `Change per-role node resources and roll the change out one node at a
time. Masters are etcd-health-gated before and after every node and applied
with an in-place-update plan gate (a VM replace is refused). Workers roll
without the etcd gate.

Sizing is per-role: the role's memory/cpu knob is updated in config and tfvars,
but each targeted apply mutates only the current node; other same-role nodes
pick up the pending change on the next full deploy. Run 'okdctl plan' after a
resize to see exactly which same-role siblings still have the change pending.

At least one of --memory-mb or --cpu is required; an omitted dimension keeps
the role's current value.

--skip-drain power-cycles the node without cordoning/draining it. The resize is
realized by a hypervisor stop→start that kills the node's pods regardless;
skipping the drain lets them restart in place on the now-roomier node instead of
evicting them cluster-wide. Prefer it when the cluster is memory-saturated, where
a drain's evicted pods cannot reschedule and the drain times out. The etcd and
Ceph health gates around the power-cycle still run.

An interrupted role roll records an op marker and resumes automatically on the
next 'okdctl node resize' of the same role or node, skipping already-completed
nodes and steps. --acknowledge-interrupted-op overrides a marker left by a
different op or node instead of refusing.`,
	Example: `  okdctl node resize masters --memory-mb 24576 --yes --confirm-cluster grappleberry
  okdctl node resize workers --memory-mb 16384 --dry-run
  okdctl node resize grappleberry-master0 --memory-mb 30720 --skip-drain --yes --confirm-cluster grappleberry`,
	Args: cobra.ExactArgs(1),
	RunE: runNodeResize,
}

var nodeAddCmd = &cobra.Command{
	Use:   "add --role worker [--count N]",
	Short: "Add worker node(s) to the cluster",
	Long: `Build and upload a per-node CoreOS ISO, revive the ignition HTTPS server for
the join window, apply a plan-gated targeted terraform create, and wait for
each new node to join and report Ready.

Only worker nodes can be added (master add/remove is not supported). --count
adds N workers in one batch, occupying the next N terraform count indices
after the persisted worker count. The ignition server is revived once for the
whole batch and torn down when the batch finishes, fails, or times out.

An interrupted add records an op marker and resumes automatically on the
next 'okdctl node add', skipping already-joined nodes and completed steps.
--acknowledge-interrupted-op overrides a marker left by a different op or
node instead of refusing. If a batch is interrupted, finish it with another
'okdctl node add' before running 'okdctl deploy' — deploy does not consult
the op marker and a partial batch's config/tfvars undercount the workers
terraform already created, so it would destroy the in-flight node(s).`,
	Example: `  okdctl node add --role worker --yes --confirm-cluster grappleberry
  okdctl node add --role worker --count 2 --dry-run`,
	Args: cobra.NoArgs,
	RunE: runNodeAdd,
}

func init() {
	nodeRemoveCmd.Flags().BoolVarP(&nodeYes, "yes", "y", false, "skip confirmation prompt")
	nodeRemoveCmd.Flags().StringVar(&nodeConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	nodeRemoveCmd.Flags().BoolVar(&nodeDryRun, flagDryRun, false, "run guards and the plan gate without mutating anything")
	nodeRemoveCmd.Flags().BoolVar(&nodeForceStorage, "force-storage", false, "allow removal even when the worker holds rook-ceph OSDs (destroys their data disk)")
	nodeRemoveCmd.Flags().BoolVar(&nodeSkipDrain, "skip-drain", false, "skip cordon/drain (assumes the node is already evacuated)")
	nodeRemoveCmd.Flags().StringVar(&nodeDrainTimeout, "drain-timeout", "10m", "per-node drain timeout")
	nodeRemoveCmd.Flags().BoolVar(&nodeAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "override a stranded marker left by a different op or node and proceed fresh")

	nodeResizeCmd.Flags().BoolVarP(&nodeYes, "yes", "y", false, "skip confirmation prompt")
	nodeResizeCmd.Flags().StringVar(&nodeConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	nodeResizeCmd.Flags().BoolVar(&nodeDryRun, flagDryRun, false, "run gates and the plan gate without mutating anything")
	nodeResizeCmd.Flags().IntVar(&nodeResizeMemoryMB, "memory-mb", 0, "new per-node memory in MiB (0 keeps current)")
	nodeResizeCmd.Flags().IntVar(&nodeResizeCPU, "cpu", 0, "new per-node cpu cores (0 keeps current)")
	nodeResizeCmd.Flags().BoolVar(&nodeSkipDrain, "skip-drain", false, "power-cycle without cordon/drain so pods restart in place (use when a drain can't reschedule under memory pressure); etcd/Ceph gates still run")
	nodeResizeCmd.Flags().BoolVar(&nodeAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "override a stranded marker left by a different op or node and proceed fresh")

	nodeAddCmd.Flags().BoolVarP(&nodeYes, "yes", "y", false, "skip confirmation prompt")
	nodeAddCmd.Flags().StringVar(&nodeConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	nodeAddCmd.Flags().BoolVar(&nodeDryRun, flagDryRun, false, "run guards and the plan gate without mutating anything")
	nodeAddCmd.Flags().StringVar(&nodeAddRole, "role", "worker", "node role to add (only worker is supported)")
	nodeAddCmd.Flags().IntVar(&nodeAddCount, "count", 1, "number of nodes to add in this batch")
	nodeAddCmd.Flags().BoolVar(&nodeAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "override a stranded marker left by a different op or node and proceed fresh")

	nodeCmd.AddCommand(nodeRemoveCmd)
	nodeCmd.AddCommand(nodeResizeCmd)
	nodeCmd.AddCommand(nodeAddCmd)
	rootCmd.AddCommand(nodeCmd)
}

// nodeConsent carries the per-command consent state buildNodeRunner needs to
// wire the informed-confirmation flow, passed explicitly rather than read from
// shared package globals so the node and cluster commands never alias flags.
// twoStage requests the destroy-grade gate (typed cluster name + y/N) used by
// the VM-destroying verbs; resize passes false for a single y/N.
type nodeConsent struct {
	yes      bool
	dryRun   bool
	twoStage bool
}

// nodeRunnerCtx bundles the disposable resources a node op holds so RunE
// bodies can defer a single cleanup. HostTotalMiB/HostAllocatedMiB are the
// read-only Proxmox memory-budget probe results (zero when the probe was
// skipped or failed — the guard then warns instead of enforcing). captured is
// the plan the confirm/preview hook observed, reused for the completion box.
type nodeRunnerCtx struct {
	runner           *node.Runner
	release          func()
	HostTotalMiB     int
	HostAllocatedMiB int
	captured         *node.OpPlan
	dryRun           bool
}

func (n *nodeRunnerCtx) cleanup() { n.release() }

// complete prints the deploy-family completion box for a finished mutating op,
// skipping dry-runs (which already printed their own box). A declined op never
// reaches here: its RunE maps node.ErrDeclined to a clean exit before calling
// complete. The captured==nil guard is a nil-safety backstop.
func (n *nodeRunnerCtx) complete(w io.Writer, elapsed time.Duration) {
	if n.dryRun || n.captured == nil {
		return
	}
	fmt.Fprint(w, render.NodeOpComplete(n.captured, elapsed))
}

// buildNodeRunner resolves the workspace, migrates the terraform root if it
// predates node-lifecycle support, loads credentials, and wires a node.Runner
// under the project run lock. It installs the informed-confirmation hook (or the
// dry-run preview) from consent. The returned cleanup zeroizes credentials and
// releases the lock.
func buildNodeRunner(cmd *cobra.Command, cfg *config.Config, verb string, consent nodeConsent, probeHost bool) (*nodeRunnerCtx, error) {
	ctx := cmd.Context()
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return nil, err
	}
	if err := ensureNodeOpsWorkspace(ctx, projectRoot, consent.yes, consent.dryRun); err != nil {
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

	tfEnv := cfg.TerraformEnvName()
	terraformDir := system.TerraformEnvDir(projectRoot, tfEnv)
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

	runner := node.NewRunner(cl, tf, cfg,
		node.WithProjectRoot(projectRoot),
		node.WithConfigPath(cfgFile),
		node.WithTerraformEnv(tfEnv),
		node.WithRunID(tui.RunID()),
		node.WithLogger(tui.SimpleLogger()))
	runner.DryRun = consent.dryRun
	runner.Reporter = func(desc string) func() { return tui.StartSpinner(ctx, desc) }

	// Wired unconditionally: construction is cheap (no I/O) and only node add
	// dereferences ISO/Ignition/SetupOpts, so every other verb simply ignores them.
	setupExec := executor.New(executor.WithWorkDir(projectRoot))
	setupPhase := setup.New(phase.WithExecutor(setupExec), phase.WithLogger(tui.SimpleLogger()))
	setupOpts := setup.NewOptions(cfg, projectRoot)
	runner.ISO = setupPhase
	runner.Ignition = setupPhase
	runner.SetupOpts = &setupOpts

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

	rc := &nodeRunnerCtx{
		runner:           runner,
		HostTotalMiB:     hostTotalMiB,
		HostAllocatedMiB: hostAllocatedMiB,
		dryRun:           consent.dryRun,
		release: func() {
			lock.Release()
			tf.ZeroizeEnv()
			creds.Zeroize()
		},
	}
	if consent.dryRun {
		out := cmd.OutOrStdout()
		runner.Preview = func(plan *node.OpPlan) {
			rc.captured = plan
			fmt.Fprint(out, render.NodeOpDryRun(plan))
		}
	} else {
		runner.Confirm = nodeConfirmHook(rc, consent, cfg.Cluster.Name, cmd.ErrOrStderr())
	}
	return rc, nil
}

// nodeConfirmHook builds the guards-before-prompt callback: it always prints the
// informed box (so --yes still surfaces the blast radius), then runs the gate
// unless --yes was passed. It records the plan for the completion box. The box
// and prompt share the stderr stream (errW) so they never interleave with piped
// stdout data, and the hook is invoked with no spinner span open.
func nodeConfirmHook(rc *nodeRunnerCtx, consent nodeConsent, clusterName string, errW io.Writer) node.ConfirmFunc {
	return func(ctx context.Context, plan *node.OpPlan) (bool, error) {
		rc.captured = plan
		fmt.Fprint(errW, render.NodeOpConfirm(plan))
		if consent.yes {
			return true, nil
		}
		return runNodeGate(ctx, consent.twoStage, clusterName)
	}
}

// runNodeGate runs the interactive consent gate: destroy-grade verbs
// (twoStage) require the operator to type the exact cluster name before the
// final y/N; resize needs only the y/N.
func runNodeGate(ctx context.Context, twoStage bool, clusterName string) (bool, error) {
	if twoStage {
		nameOK, err := promptForClusterNameConfirmation(ctx, clusterName,
			fmt.Sprintf("type cluster name %q to confirm: ", clusterName))
		if err != nil || !nameOK {
			return false, err
		}
	}
	return promptForConfirmation(ctx, "proceed? [y/N]: ")
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
		tui.LF("host_node", probe.Node),
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
// dryRun refuses instead: --dry-run promises no mutation, so a root needing
// migration must be reported and left untouched rather than rewritten or
// prompted for mid-preview.
func ensureNodeOpsWorkspace(ctx context.Context, projectRoot string, yes, dryRun bool) error {
	ok, err := deploy.TerraformRootSupportsNodeOps(projectRoot)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if dryRun {
		return &errtypes.UsageError{Msg: "terraform root needs node-ops migration; re-run without --dry-run to migrate (no changes made)"}
	}
	format, stamped, err := deploy.TerraformRootFormat(projectRoot)
	if err != nil {
		return err
	}
	if stamped {
		tui.Warn("terraform root predates node-lifecycle support",
			tui.LF("workspace_format", format),
			tui.LF("binary_expects", deploy.ExpectedTerraformRootFormat()))
	} else {
		tui.Warn("terraform root predates node-lifecycle support; it must be migrated (worker_count / master sizing variables)")
	}
	preview, err := deploy.PreviewTerraformRootMigration(projectRoot)
	if err != nil {
		return err
	}
	for _, f := range preview.OperatorModified {
		tui.Warn("you edited this terraform file; your version will be backed up before overwrite", tui.LF("path", f))
	}
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

	consent := nodeConsent{yes: nodeYes, dryRun: nodeDryRun, twoStage: true}
	rc, err := buildNodeRunner(cmd, cfg, "remove", consent, false)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	start := time.Now()
	if err := rc.runner.RemoveWorker(cmd.Context(), target, node.RemoveOptions{
		ForceStorage: nodeForceStorage,
		SkipDrain:    nodeSkipDrain,
		DrainTimeout: nodeDrainTimeout,
		Acknowledge:  nodeAcknowledgeInterrupted,
	}); err != nil {
		if errors.Is(err, node.ErrDeclined) {
			return nil
		}
		return err
	}
	rc.complete(cmd.OutOrStdout(), time.Since(start))
	return nil
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
	if err := validateResizeFlags(nodeResizeMemoryMB, nodeResizeCPU); err != nil {
		return err
	}

	if err := confirmClusterMatches(nodeYes, nodeConfirmCluster, cfg.Cluster.Name, "node resize"); err != nil {
		return err
	}

	consent := nodeConsent{yes: nodeYes, dryRun: nodeDryRun, twoStage: false}
	rc, err := buildNodeRunner(cmd, cfg, "resize", consent, true)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	start := time.Now()
	if err := rc.runner.Resize(cmd.Context(), scope, node.ResizeOptions{
		MemoryMB:         nodeResizeMemoryMB,
		CPU:              nodeResizeCPU,
		HostTotalMiB:     rc.HostTotalMiB,
		HostAllocatedMiB: rc.HostAllocatedMiB,
		Acknowledge:      nodeAcknowledgeInterrupted,
		SkipDrain:        nodeSkipDrain,
	}); err != nil {
		if errors.Is(err, node.ErrDeclined) {
			return nil
		}
		return err
	}
	rc.complete(cmd.OutOrStdout(), time.Since(start))
	return nil
}

func runNodeAdd(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}
	if err := validateAddFlags(nodeAddRole, nodeAddCount); err != nil {
		return err
	}

	if err := confirmClusterMatches(nodeYes, nodeConfirmCluster, cfg.Cluster.Name, "node add"); err != nil {
		return err
	}

	consent := nodeConsent{yes: nodeYes, dryRun: nodeDryRun, twoStage: false}
	rc, err := buildNodeRunner(cmd, cfg, "add", consent, true)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	start := time.Now()
	if err := rc.runner.AddWorkers(cmd.Context(), node.AddOptions{
		Count:            nodeAddCount,
		HostTotalMiB:     rc.HostTotalMiB,
		HostAllocatedMiB: rc.HostAllocatedMiB,
		Acknowledge:      nodeAcknowledgeInterrupted,
	}); err != nil {
		if errors.Is(err, node.ErrDeclined) {
			return nil
		}
		return err
	}
	rc.complete(cmd.OutOrStdout(), time.Since(start))
	return nil
}

// validateAddFlags requires --role worker (master add/remove is not
// supported, mirroring remove's guard) and a positive --count.
func validateAddFlags(role string, count int) error {
	if role != nodetypes.RoleWorker.String() {
		return &errtypes.UsageError{Msg: fmt.Sprintf("--role %q is not supported; only worker nodes can be added (master add/remove is not supported)", role)}
	}
	if count < 1 {
		return &errtypes.UsageError{Msg: "--count must be >= 1"}
	}
	return nil
}

// validateResizeFlags requires at least one resize dimension: an omitted
// --memory-mb or --cpu keeps the role's current value rather than erroring.
func validateResizeFlags(memoryMB, cpu int) error {
	if memoryMB <= 0 && cpu <= 0 {
		return &errtypes.UsageError{Msg: "resize requires at least one of --memory-mb or --cpu"}
	}
	return nil
}

func parseResizeScope(arg string) (node.ResizeScope, error) {
	switch arg {
	case "masters", nodetypes.RoleMaster.String():
		return node.ResizeScope{Role: nodetypes.RoleMaster}, nil
	case "workers", nodetypes.RoleWorker.String():
		return node.ResizeScope{Role: nodetypes.RoleWorker}, nil
	default:
		if _, ok := cluster.NodeIndex(arg); !ok {
			return node.ResizeScope{}, &errtypes.UsageError{Msg: fmt.Sprintf("resize target %q must be 'masters', 'workers', or a node name with a numeric suffix", arg)}
		}
		return node.ResizeScope{Node: arg}, nil
	}
}
