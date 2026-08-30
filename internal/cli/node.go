package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/clusterstatus"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/runlock"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/workspace"
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
	nodeResizeOSDiskGB         int
	nodeAcknowledgeInterrupted bool
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
	Use:   "resize (masters|workers|<name>)",
	Short: "Resize node CPU/memory/OS-disk per role, rolled out one node at a time",
	Long: `Change per-role node resources and roll the change out one node at a
time. Masters are etcd-health-gated before and after every node and applied
with an in-place-update plan gate (a VM replace is refused). Workers roll
without the etcd gate.

Sizing is per-role: the role's memory/cpu/disk knob is updated in config and
tfvars, but each targeted apply mutates only the current node; other same-role
nodes pick up the pending change on the next full deploy. Run 'okdctl plan'
after a resize to see exactly which same-role siblings still have the change
pending.

At least one of --memory-mb, --cpu, or --os-disk-gb is required; an omitted
dimension keeps the role's current value. --os-disk-gb is grow-only (shrink is
refused) and, unlike memory/cpu, is realized live: the Proxmox disk is grown
and the in-guest filesystem grown into it via 'oc debug' without a
power-cycle. A resize that combines --os-disk-gb with --memory-mb/--cpu still
grows the disk live before the power-cycle that realizes the other dimensions.
--os-disk-gb is role-scoped only ('masters'/'workers', not a single node
name): a same-role sibling can always catch up on a memory/cpu change at its
next full deploy, but CoreOS only grows the filesystem on firstboot, so a
sibling left behind by a single-node disk grow could never catch up, and a
later same-size role-wide resize would then be refused by the grow-only check
above.

--skip-drain power-cycles the node without cordoning/draining it. The resize is
realized by a hypervisor stop→start that kills the node's pods regardless;
skipping the drain lets them restart in place on the now-roomier node instead of
evicting them cluster-wide. Prefer it when the cluster is memory-saturated, where
a drain's evicted pods cannot reschedule and the drain times out. The etcd and
Ceph health gates around the power-cycle still run. --skip-drain has no effect
on a disk-only resize, which never power-cycles.

An interrupted role roll records an op marker and resumes automatically on the
next 'okdctl node resize' of the same role or node, skipping already-completed
nodes and steps. --acknowledge-interrupted-op overrides a marker left by a
different op or node instead of refusing.`,
	Example: `  okdctl node resize masters --memory-mb 24576 --yes --confirm-cluster grappleberry
  okdctl node resize workers --memory-mb 16384 --dry-run
  okdctl node resize grappleberry-master0 --memory-mb 30720 --skip-drain --yes --confirm-cluster grappleberry
  okdctl node resize masters --os-disk-gb 100`,
	Args: cobra.ExactArgs(1),
	RunE: runNodeResize,
}

var nodeAddCmd = &cobra.Command{
	Use:   "add [--count N]",
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
node instead of refusing. While the marker is in flight, 'okdctl deploy'
refuses to run (a partial batch's config/tfvars undercount the workers
terraform already created) unless it too is passed
--acknowledge-interrupted-op.`,
	Example: `  okdctl node add --yes --confirm-cluster grappleberry
  okdctl node add --count 2 --dry-run`,
	// revives the root-only ignition HTTPS server; elevation gate re-execs under sudo unless --dry-run
	Annotations: map[string]string{annotationKeyRequiresRoot: annotationValueTrue},
	Args:        cobra.NoArgs,
	RunE:        runNodeAdd,
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
	nodeResizeCmd.Flags().IntVar(&nodeResizeOSDiskGB, "os-disk-gb", 0, "grow the role's OS disk to this size in GiB (grow-only, role-scoped only — 'masters'/'workers', not a single node; disk-only resizes are live, no power-cycle)")
	nodeResizeCmd.Flags().BoolVar(&nodeSkipDrain, "skip-drain", false, "power-cycle without cordon/drain so pods restart in place (use when a drain can't reschedule under memory pressure); etcd/Ceph gates still run")
	nodeResizeCmd.Flags().BoolVar(&nodeAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "override a stranded marker left by a different op or node and proceed fresh")

	nodeAddCmd.Flags().BoolVarP(&nodeYes, "yes", "y", false, "skip confirmation prompt")
	nodeAddCmd.Flags().StringVar(&nodeConfirmCluster, "confirm-cluster", "", "required with --yes; must equal the config cluster name")
	nodeAddCmd.Flags().BoolVar(&nodeDryRun, flagDryRun, false, "run guards and the plan gate without mutating anything")
	nodeAddCmd.Flags().IntVar(&nodeAddCount, "count", 1, "number of nodes to add in this batch")
	nodeAddCmd.Flags().BoolVar(&nodeAcknowledgeInterrupted, "acknowledge-interrupted-op", false, "override a stranded marker left by a different op or node and proceed fresh")

	nodeCmd.AddCommand(nodeRemoveCmd)
	nodeCmd.AddCommand(nodeResizeCmd)
	nodeCmd.AddCommand(nodeAddCmd)
	nodeCmd.AddCommand(nodeManageCmd)
	rootCmd.AddCommand(nodeCmd)
}

// nodeConsent is explicit (not package globals) so commands can't alias flags;
// twoStage selects the destroy-grade typed-name+y/N gate.
type nodeConsent struct {
	yes      bool
	dryRun   bool
	twoStage bool
}

// destroyGradeVerbs is the single source of truth for which verbs need the
// destroy-grade gate, so a downgrade can't happen via an inline literal at a
// RunE call site.
var destroyGradeVerbs = map[string]bool{
	"remove":  true,
	"compact": true,
}

// nodeRunnerCtx bundles disposable node-op resources so RunE can defer a
// single cleanup; zero Host*MiB/DatastoreAvailGB means the probe was
// skipped or failed and the guards warn instead of enforcing.
type nodeRunnerCtx struct {
	runner           *node.Runner
	release          func()
	HostTotalMiB     int
	HostAllocatedMiB int
	DatastoreAvailGB int
	captured         *node.OpPlan
	dryRun           bool
}

// cleanup releases via the field so a later rewrap by buildNodeRunner is honoured.
func (n *nodeRunnerCtx) cleanup() { n.release() }

// complete prints the completion box for a finished op, skipping dry-runs; a
// declined op never reaches here (RunE maps ErrDeclined to a clean exit first).
func (n *nodeRunnerCtx) complete(w io.Writer, elapsed time.Duration) {
	if n.dryRun || n.captured == nil {
		return
	}
	fmt.Fprint(w, render.NodeOpComplete(n.captured, elapsed))
}

// nodeOpsEnv is the pre-TUI environment for node ops; it owns credentials
// (close zeroizes them), split from runner construction so the wizard can hold
// one env across screens.
type nodeOpsEnv struct {
	projectRoot      string
	creds            *credentials.ProxmoxCredentials
	tfEnv            string
	hostTotalMiB     int
	hostAllocatedMiB int
	datastoreAvailGB int
}

// close zeroizes credentials and chowns the workdir back to the invoking user
// (node add's sudo re-exec leaves it root-owned); a no-op when SUDO_UID is
// unset.
func (e *nodeOpsEnv) close() {
	defer e.creds.Zeroize()
	workDir := filepath.Join(e.projectRoot, workspace.WorkDirName)
	if err := system.ChownTreeToInvokingUser(workDir); err != nil {
		logutil.Warn("workdir chown back to user incomplete", logutil.LF("err", err))
	}
}

// prepareNodeOpsEnv resolves the workspace, loads credentials, and probes host
// memory — everything that must happen before a TUI takes the terminal.
func prepareNodeOpsEnv(ctx context.Context, cfg *config.Config, probeHost bool) (*nodeOpsEnv, error) {
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return nil, err
	}
	tfEnv := cfg.TerraformEnvName()
	terraformDir := workspace.TerraformEnvDir(projectRoot, tfEnv)
	if err := ensureTerraformWorkspace(terraformDir); err != nil {
		return nil, err
	}

	creds, err := handleCredentials(cfg)
	if err != nil {
		return nil, err
	}

	var hostTotalMiB, hostAllocatedMiB, datastoreAvailGB int
	if probeHost && creds.IsValid() {
		hostTotalMiB, hostAllocatedMiB, datastoreAvailGB = runHostBudgetProbe(ctx, cfg, creds)
	}

	return &nodeOpsEnv{
		projectRoot:      projectRoot,
		creds:            creds,
		tfEnv:            cfg.TerraformEnvName(),
		hostTotalMiB:     hostTotalMiB,
		hostAllocatedMiB: hostAllocatedMiB,
		datastoreAvailGB: datastoreAvailGB,
	}, nil
}

// buildNodeRunner composes prepareNodeOpsEnv and newRunner, folding the env's
// lifetime into the returned cleanup so callers keep a single defer.
func buildNodeRunner(cmd *cobra.Command, cfg *config.Config, verb string, consent nodeConsent, probeHost bool) (*nodeRunnerCtx, error) {
	env, err := prepareNodeOpsEnv(cmd.Context(), cfg, probeHost)
	if err != nil {
		return nil, err
	}
	rc, err := env.newRunner(cmd, cfg, verb, consent, logutil.SimpleLogger(), nil)
	if err != nil {
		env.close()
		return nil, err
	}
	inner := rc.release
	rc.release = func() { inner(); env.close() }
	return rc, nil
}

// newRunner wires a node.Runner under the run lock, routing log/subprocOut away
// from stderr when the manage TUI owns the terminal; the returned cleanup
// releases the lock and zeroizes the terraform env, but not credentials (owned
// by nodeOpsEnv).
func (e *nodeOpsEnv) newRunner(cmd *cobra.Command, cfg *config.Config, verb string, consent nodeConsent, log *slog.Logger, subprocOut io.Writer) (*nodeRunnerCtx, error) {
	ctx := cmd.Context()
	creds := e.creds

	terraformDir := workspace.TerraformEnvDir(e.projectRoot, e.tfEnv)
	tfOpts := []terraform.Option{terraform.WithLogger(log)}
	if creds.IsValid() {
		tfOpts = append(tfOpts, terraform.WithEnv(creds.Env()))
	}
	tf := terraform.New(terraformDir, tfOpts...)

	cl, err := clusterstatus.NewClient(e.projectRoot)
	if err != nil {
		tf.ZeroizeEnv()
		return nil, err
	}

	lock, err := runlock.Acquire(e.projectRoot, "node "+verb)
	if err != nil {
		tf.ZeroizeEnv()
		return nil, err
	}

	runner := node.NewRunner(cl, tf, cfg,
		node.WithProjectRoot(e.projectRoot),
		node.WithConfigPath(cfgFile),
		node.WithTerraformEnv(e.tfEnv),
		node.WithRunID(logutil.RunID()),
		node.WithLogger(log))
	runner.DryRun = consent.dryRun
	runner.Reporter = func(desc string) func() { return tui.StartSpinner(ctx, desc) }
	runner.Disk = &node.DebugNodeGrower{Runner: cl}

	// Wired unconditionally (cheap, no I/O); only node add dereferences ISO/Ignition/Provision.
	provExecOpts := []executor.Option{executor.WithWorkDir(e.projectRoot)}
	if subprocOut != nil {
		provExecOpts = append(provExecOpts, executor.WithStdout(subprocOut), executor.WithStderr(subprocOut))
	}
	provExec := executor.New(provExecOpts...)
	prov := provision.New(phase.WithExecutor(provExec), phase.WithLogger(log))
	runner.ISO = prov
	runner.Ignition = prov
	runner.Provision = provision.NewOptions(e.projectRoot)

	// A resize takes effect only after a power-cycle (bpg/proxmox changes VM
	// config without restarting); resize fails safe when Power is nil.
	if creds.IsValid() {
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
		HostTotalMiB:     e.hostTotalMiB,
		HostAllocatedMiB: e.hostAllocatedMiB,
		DatastoreAvailGB: e.datastoreAvailGB,
		dryRun:           consent.dryRun,
		release: func() {
			lock.Release()
			tf.ZeroizeEnv()
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

// nodeConfirmHook always prints the informed box (even under --yes, so blast
// radius is visible) before running the gate; box and prompt share stderr so
// they never interleave with piped stdout.
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

// runNodeGate runs the typed-name+y/N gate for destroy-grade (twoStage) verbs,
// or just y/N otherwise.
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

// runHostBudgetProbe reads host memory and os-datastore headroom for the
// budget guards; a probe failure degrades to warn-only (zeros) instead of
// blocking a resize.
func runHostBudgetProbe(ctx context.Context, cfg *config.Config, creds *credentials.ProxmoxCredentials) (totalMiB, allocatedMiB, datastoreAvailGB int) {
	px := cfg.Provider.Proxmox
	if px == nil {
		return 0, 0, 0
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
		logutil.Warn("host memory-budget probe failed; memory guard will warn instead of enforce", logutil.LF("err", err))
		return 0, 0, 0
	}
	logutil.Info("host memory budget",
		logutil.LF("host_node", probe.Node),
		logutil.LF("total_mib", probe.HostMemTotalMiB()),
		logutil.LF("allocated_mib", probe.GuestAllocatedMiB()))
	for _, d := range probe.Datastores {
		logutil.Info("datastore headroom",
			logutil.LF("name", d.Name),
			logutil.LF("free_gib", d.AvailBytes/(1024*1024*1024)),
			logutil.LF("total_gib", d.TotalBytes/(1024*1024*1024)))
		if d.Name == px.Storage {
			datastoreAvailGB = int(d.AvailBytes / (1024 * 1024 * 1024)) //nolint:gosec // G115: GiB-scale value fits int
		}
	}
	return probe.HostMemTotalMiB(), probe.GuestAllocatedMiB(), datastoreAvailGB
}

// ensureTerraformWorkspace fails fast (before credentials/run lock) so a
// never-deployed directory doesn't surface as a raw terraform chdir failure.
func ensureTerraformWorkspace(terraformDir string) error {
	if _, err := os.Stat(terraformDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &errtypes.ConfigError{Msg: fmt.Sprintf("no terraform workspace at %s; run 'okdctl deploy' from this directory first", terraformDir)}
		}
		return &errtypes.ConfigError{Msg: "stat terraform workspace", Err: err}
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

	consent := nodeConsent{yes: nodeYes, dryRun: nodeDryRun, twoStage: destroyGradeVerbs["remove"]}
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
	if err := validateResizeFlags(nodeResizeMemoryMB, nodeResizeCPU, nodeResizeOSDiskGB); err != nil {
		return err
	}

	if err := confirmClusterMatches(nodeYes, nodeConfirmCluster, cfg.Cluster.Name, "node resize"); err != nil {
		return err
	}

	consent := nodeConsent{yes: nodeYes, dryRun: nodeDryRun, twoStage: destroyGradeVerbs["resize"]}
	rc, err := buildNodeRunner(cmd, cfg, "resize", consent, true)
	if err != nil {
		return err
	}
	defer rc.cleanup()

	start := time.Now()
	if err := rc.runner.Resize(cmd.Context(), scope, node.ResizeOptions{
		MemoryMB:         nodeResizeMemoryMB,
		CPU:              nodeResizeCPU,
		OSDiskGB:         nodeResizeOSDiskGB,
		HostTotalMiB:     rc.HostTotalMiB,
		HostAllocatedMiB: rc.HostAllocatedMiB,
		DatastoreAvailGB: rc.DatastoreAvailGB,
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
	if err := validateAddFlags(nodeAddCount); err != nil {
		return err
	}

	if err := confirmClusterMatches(nodeYes, nodeConfirmCluster, cfg.Cluster.Name, "node add"); err != nil {
		return err
	}

	consent := nodeConsent{yes: nodeYes, dryRun: nodeDryRun, twoStage: destroyGradeVerbs["add"]}
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

func validateAddFlags(count int) error {
	if count < 1 {
		return &errtypes.UsageError{Msg: "--count must be >= 1"}
	}
	return nil
}

// validateResizeFlags treats 0 as omitted, keeping the role's current value.
func validateResizeFlags(memoryMB, cpu, osDiskGB int) error {
	if memoryMB <= 0 && cpu <= 0 && osDiskGB <= 0 {
		return &errtypes.UsageError{Msg: "resize requires at least one of --memory-mb, --cpu, or --os-disk-gb"}
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
