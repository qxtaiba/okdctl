package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// Default gates: NodeReadyTimeout bounds a node's Ready wait; EtcdGateTimeout
// bounds the pre/post etcd-quorum gate.
const (
	DefaultNodeReadyTimeout = 15 * time.Minute
	DefaultEtcdGateTimeout  = 10 * time.Minute
	// DefaultCephGateTimeout is generous: an OSD rebalance can take a while to reach active+clean.
	DefaultCephGateTimeout = 30 * time.Minute
	// DefaultClusterReadyTimeout bounds cluster start's Ready wait, ticking
	// ApprovePendingCSRs each poll so cert rotation doesn't stall it.
	DefaultClusterReadyTimeout = 30 * time.Minute
	// DefaultSnapshotTaskTimeout bounds a snapshot create/rollback/delete's
	// wait for its async pvesh task.
	DefaultSnapshotTaskTimeout = 5 * time.Minute
	// hostMemoryReserveMiB is hypervisor headroom (ZFS ARC, host services) the
	// memory-budget guard keeps free.
	hostMemoryReserveMiB = 2048
)

// clusterClient is the slice of cluster.Client node ops drive, as an interface
// so tests can substitute a call-recording fake.
type clusterClient interface {
	ListNodes(ctx context.Context) ([]cluster.NodeDetail, error)
	Cordon(ctx context.Context, node string) error
	Uncordon(ctx context.Context, node string) error
	Drain(ctx context.Context, node string, opts cluster.DrainOptions) error
	DeleteNode(ctx context.Context, node string) error
	EtcdHealthy(ctx context.Context) (cluster.EtcdHealth, error)
	CephHealthy(ctx context.Context) (cluster.CephHealth, error)
	MastersSchedulable(ctx context.Context) (bool, error)
	SetMastersSchedulable(ctx context.Context, schedulable bool) error
	PodsForSelector(ctx context.Context, namespace, selector string) ([]cluster.PodPlacement, error)
	Apply(ctx context.Context, manifest []byte) error
	ApprovePendingCSRs(ctx context.Context) (int, error)
	SignerNotAfter(ctx context.Context) (time.Time, error)
}

// vmPowerCycler stops→starts a VM via the Proxmox API; nil (no credentials)
// makes resize refuse rather than leave a change unrealized.
type vmPowerCycler interface {
	PowerCycleVM(ctx context.Context, node string, vmid int) error
	ShutdownVM(ctx context.Context, node string, vmid int) error
	StartVM(ctx context.Context, node string, vmid int) error
}

// diskGrower grows a node's OS filesystem into a freshly-grown virtual disk
// (SCSI rescan, growpart, xfs_growfs) in-guest via `oc debug`; nil refuses a
// disk resize up front rather than applying a grow the guest never realizes.
type diskGrower interface {
	GrowOSDisk(ctx context.Context, node string) error
}

// snapshotClient mirrors hostssh's pvesh-backed snapshot primitives as an
// interface, so tests can substitute a call-recording fake.
type snapshotClient interface {
	CreateSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name, description string, timeout time.Duration) error
	ListSnapshots(ctx context.Context, p *hostssh.RemoteISOParams, vmid int) ([]hostssh.SnapshotInfo, error)
	RollbackSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name string, timeout time.Duration) error
	DeleteSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name string, timeout time.Duration) error
	VMAgentEnabled(ctx context.Context, p *hostssh.RemoteISOParams, vmid int) (bool, error)
}

// HostsshSnapshotClient is the production snapshotClient, delegating to package
// hostssh's pvesh primitives.
type HostsshSnapshotClient struct{}

// CreateSnapshot implements snapshotClient via hostssh.CreateSnapshot.
func (HostsshSnapshotClient) CreateSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name, description string, timeout time.Duration) error {
	return hostssh.CreateSnapshot(ctx, p, vmid, name, description, timeout)
}

// ListSnapshots implements snapshotClient via hostssh.ListSnapshots.
func (HostsshSnapshotClient) ListSnapshots(ctx context.Context, p *hostssh.RemoteISOParams, vmid int) ([]hostssh.SnapshotInfo, error) {
	return hostssh.ListSnapshots(ctx, p, vmid)
}

// RollbackSnapshot implements snapshotClient via hostssh.RollbackSnapshot.
func (HostsshSnapshotClient) RollbackSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name string, timeout time.Duration) error {
	return hostssh.RollbackSnapshot(ctx, p, vmid, name, timeout)
}

// DeleteSnapshot implements snapshotClient via hostssh.DeleteSnapshot.
func (HostsshSnapshotClient) DeleteSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name string, timeout time.Duration) error {
	return hostssh.DeleteSnapshot(ctx, p, vmid, name, timeout)
}

// VMAgentEnabled implements snapshotClient via hostssh.VMAgentEnabled.
func (HostsshSnapshotClient) VMAgentEnabled(ctx context.Context, p *hostssh.RemoteISOParams, vmid int) (bool, error) {
	return hostssh.VMAgentEnabled(ctx, p, vmid)
}

// terraformExec is the slice of terraform.Executor node ops drive; an interface
// so a fake can record plan/apply calls without running terraform.
type terraformExec interface {
	Init(ctx context.Context) error
	Plan(ctx context.Context, opts terraform.PlanOptions) error
	ShowPlanChanges(ctx context.Context, planFile string) ([]terraform.ResourceChange, error)
	SnapshotState(ctx context.Context) (string, error)
	Apply(ctx context.Context, opts terraform.ApplyOptions) error
	WithLockHint(err error) error
	StateHasResource(ctx context.Context, addr string) (bool, error)
}

// isoProvisioner is the slice of provision.Provisioner node add drives to
// build/upload the ISO; an interface so tests avoid shelling out.
type isoProvisioner interface {
	BuildCustomISOs(ctx context.Context, cfg *config.Config, opts provision.Options) error
	UploadCustomISOsToProxmox(ctx context.Context, cfg *config.Config, opts provision.Options) error
}

// ignitionServer exposes worker.ign over HTTPS for the join window; a teardown
// error means httpd may still be serving the pull secret — callers must surface
// it loudly.
type ignitionServer interface {
	ReviveIgnitionServer(ctx context.Context, cfg *config.Config, projectRoot, clusterDir string) error
	TeardownIgnitionServer(ctx context.Context) error
}

// Runner drives node-lifecycle ops against one cluster: TF mutates VMs, Cluster
// runs the Kubernetes lifecycle, and ConfigPath persists topology for later deploys
// to reconcile against. projectRoot/workDir/envDir are unexported so NewRunner's
// derive-from-root invariant can't be skewed by a later field assignment.
type Runner struct {
	Cluster     clusterClient
	TF          terraformExec
	Cfg         *config.Config
	ConfigPath  string
	projectRoot string
	workDir     string
	envDir      string
	RunID       string
	DryRun      bool
	Log         *slog.Logger
	Reporter    logutil.ProgressReporter

	// Power performs the post-resize power-cycle; nil (no Proxmox credentials) makes resize fail safe.
	Power vmPowerCycler

	// Disk realizes an OS-disk grow inside the guest; nil fails disk resizes closed, mirroring Power.
	Disk diskGrower

	// Proxmox carries the pvesh-over-SSH params snapshot ops use; nil fails
	// closed the same way Power does for resize.
	Proxmox *hostssh.RemoteISOParams

	// Snapshot is the pvesh-backed client snapshot ops drive; an interface so
	// tests can record calls without a live hypervisor.
	Snapshot snapshotClient

	// SnapshotTaskTimeout bounds a snapshot create/rollback/delete's wait for its async pvesh task.
	SnapshotTaskTimeout time.Duration

	// ISO/Ignition drive node add's ISO build/upload and ignition-server revive
	// (zero/nil for every other op); Provision carries their artifact roots.
	ISO       isoProvisioner
	Ignition  ignitionServer
	Provision provision.Options

	// Confirm gates each mutating op before the first mutation (nil
	// auto-approves); Preview renders the dry-run plan (nil falls back to
	// slog).
	Confirm ConfirmFunc
	Preview PreviewFunc

	// OnStep, when non-nil, observes each step just before its marker is
	// written (the TUI execution-screen feed); purely observational, errors
	// never flow back.
	OnStep func(target string, step Step)

	// preConsented suppresses the confirm gate for ops composed under a
	// higher-level consent (compact's inner RemoveWorker/Resize calls); see
	// Compact.
	preConsented bool

	NodeReadyTimeout    time.Duration
	EtcdGateTimeout     time.Duration
	CephGateTimeout     time.Duration
	ClusterReadyTimeout time.Duration

	// tfEnv is the terraform env name from WithTerraformEnv; NewRunner resolves
	// it into envDir after all options apply, so order doesn't matter.
	tfEnv string
}

// RunnerOption configures optional Runner wiring in NewRunner, mirroring the
// okd.New/addon.NewManager option style.
type RunnerOption func(*Runner)

// WithProjectRoot sets the project root the work and terraform env directories derive from.
func WithProjectRoot(root string) RunnerOption {
	return func(r *Runner) { r.projectRoot = root }
}

// WithConfigPath sets the okdctl.yaml path each op persists topology to.
func WithConfigPath(path string) RunnerOption {
	return func(r *Runner) { r.ConfigPath = path }
}

// WithTerraformEnv selects the terraform environment whose directory node ops plan and apply in.
func WithTerraformEnv(env string) RunnerOption {
	return func(r *Runner) { r.tfEnv = env }
}

// WithRunID tags the runner's op markers and logs with the CLI run id.
func WithRunID(id string) RunnerOption {
	return func(r *Runner) { r.RunID = id }
}

// WithLogger sets the runner's logger; nil is normalized to a no-op logger.
func WithLogger(log *slog.Logger) RunnerOption {
	return func(r *Runner) { r.Log = log }
}

// NewRunner wires a Runner with derived directories and default timeouts; core
// collaborators are positional, everything else via options.
func NewRunner(cl *cluster.Client, tf *terraform.Executor, cfg *config.Config, opts ...RunnerOption) *Runner {
	r := &Runner{
		Cluster:             cl,
		TF:                  tf,
		Cfg:                 cfg,
		Reporter:            logutil.NopProgressReporter,
		Snapshot:            HostsshSnapshotClient{},
		NodeReadyTimeout:    DefaultNodeReadyTimeout,
		EtcdGateTimeout:     DefaultEtcdGateTimeout,
		CephGateTimeout:     DefaultCephGateTimeout,
		ClusterReadyTimeout: DefaultClusterReadyTimeout,
		SnapshotTaskTimeout: DefaultSnapshotTaskTimeout,
	}
	for _, opt := range opts {
		opt(r)
	}
	r.Log = logutil.OrNop(r.Log)
	r.workDir = workspace.WorkDir(r.projectRoot)
	r.envDir = workspace.TerraformEnvDir(r.projectRoot, r.tfEnv)
	return r
}

func (r *Runner) marker() string { return markerPath(r.workDir) }

// mark notifies OnStep and writes the op marker for a mutating step.
func (r *Runner) mark(op Op, target string, step Step) error {
	if r.OnStep != nil {
		r.OnStep(target, step)
	}
	return markStep(r.marker(), op, target, step, r.RunID, r.Cfg.Cluster.Name)
}

// startProgress starts r.Reporter for desc unless dry-run (also silences gates
// ahead of the dry-run branch, e.g. compact's preflight etcd check); nil-safe
// for direct Runner values.
func (r *Runner) startProgress(desc string) (stop func()) {
	if r.DryRun {
		return func() {}
	}
	if r.Reporter == nil {
		return logutil.NopProgressReporter(desc)
	}
	return r.Reporter(desc)
}

// persistTopology saves okdctl.yaml before re-rendering tfvars, so a crash
// leaves config ahead of tfvars for reconciliation; WriteTerraformVars
// preserves the bootstrap sentinel so re-render can't resurrect the bootstrap
// VM.
func (r *Runner) persistTopology() error {
	if err := config.NewLoader().Save(r.Cfg, r.ConfigPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := provision.WriteTerraformVars(r.Cfg, r.envDir); err != nil {
		return fmt.Errorf("render terraform vars: %w", err)
	}
	return nil
}

// nodeOpPlanVars merges planVars onto post-deploy invariants (bootstrap gone,
// workers running) as -var overrides, so stale tfvars or module defaults can't
// sneak into the targeted plan.
func nodeOpPlanVars(planVars map[string]string) map[string]string {
	vars := map[string]string{
		"bootstrap_enabled":         "false",
		"start_workers_immediately": "true",
	}
	maps.Copy(vars, planVars)
	return vars
}

// planTargeted plans (address, want) with node-op invariant vars; callers MUST
// invoke the returned cleanup after any apply. An empty plan means refused, or
// (only when resuming) already-landed — a delete is classified unconditionally;
// on a fresh run an empty update plan is a fatal -var regression, not success.
func (r *Runner) planTargeted(ctx context.Context, address string, want terraform.PlanAction, planVars map[string]string, resuming bool) (planPath string, alreadyAtTarget bool, cleanup func(), err error) {
	noop := func() {}
	if err := r.TF.Init(ctx); err != nil {
		return "", false, noop, r.TF.WithLockHint(&errtypes.ClusterError{Msg: "terraform init", Err: err})
	}

	planFile := "node-op.tfplan"
	planPath = filepath.Join(r.envDir, planFile)
	cleanup = func() {
		if rmErr := system.SafeRemove(planPath); rmErr != nil {
			r.Log.Warn("node: plan file cleanup failed", "err", rmErr)
		}
	}

	if planErr := r.TF.Plan(ctx, terraform.PlanOptions{
		OutputPlanFile: planFile,
		Targets:        []string{address},
		Vars:           nodeOpPlanVars(planVars),
	}); planErr != nil {
		cleanup()
		return "", false, noop, r.TF.WithLockHint(&errtypes.ClusterError{Msg: "terraform plan", Err: planErr})
	}

	changes, showErr := r.TF.ShowPlanChanges(ctx, planPath)
	if showErr != nil {
		cleanup()
		return "", false, noop, &errtypes.ClusterError{Msg: "read terraform plan", Err: showErr}
	}
	if gateErr := terraform.AssertOnlyChange(changes, address, want); gateErr != nil {
		if len(changes) == 0 {
			inState, stateErr := r.TF.StateHasResource(ctx, address)
			if stateErr != nil {
				cleanup()
				return "", false, noop, &errtypes.ClusterError{Msg: "check terraform state", Err: stateErr}
			}
			if terraform.EmptyPlanMeansAlreadyAtTarget(inState, want) &&
				(want == terraform.PlanActionDelete || resuming) {
				cleanup()
				return "", true, noop, nil
			}
		}
		cleanup()
		return "", false, noop, r.TF.WithLockHint(&errtypes.ClusterError{Msg: "plan safety gate refused the change", Err: gateErr})
	}
	return planPath, false, cleanup, nil
}

// targetedApply plans, gates to (address, want), snapshots state, and applies;
// a gate failure aborts before any mutation, and planVars keep dry-run previews
// truthful without writing tfvars.
func (r *Runner) targetedApply(ctx context.Context, address string, want terraform.PlanAction, planVars map[string]string, resuming bool) error {
	stop := r.startProgress(fmt.Sprintf("applying terraform change to %s", address))
	defer stop()

	planPath, alreadyAtTarget, cleanup, err := r.planTargeted(ctx, address, want, planVars, resuming)
	if err != nil {
		return err
	}
	defer cleanup()

	if alreadyAtTarget {
		if want == terraform.PlanActionDelete && !resuming {
			// Expected after an acknowledged partial remove, but also what a lost/mismatched
			// state file produces — in which case the VM still exists and a quiet success would strand it.
			r.Log.Warn("node: terraform state has no record of this resource — treating it as already destroyed; if the vm still exists in proxmox, the workspace state is wrong and this result must not be trusted",
				"tf_address", address)
		} else {
			r.Log.Info("node: already at target — skipping apply", "tf_address", address, "action", string(want))
		}
		return nil
	}

	if r.DryRun {
		r.Log.Info("node: dry-run — plan gate passed, skipping apply", "tf_address", address, "action", string(want))
		return nil
	}

	snap, snapErr := r.TF.SnapshotState(ctx)
	if snapErr != nil {
		return &errtypes.ClusterError{Msg: "state snapshot", Err: snapErr}
	}

	if err := r.TF.Apply(ctx, terraform.ApplyOptions{PlanFile: planPath}); err != nil {
		msg := "terraform apply"
		if snap != "" {
			msg = fmt.Sprintf("terraform apply (state backup: %s)", snap)
		}
		return r.TF.WithLockHint(&errtypes.ClusterError{Msg: msg, Err: err})
	}
	return nil
}

// waitEtcdHealthy blocks until the etcd quorum is healthy or the gate times
// out; it is the mandatory pre/post gate around any control-plane mutation.
func (r *Runner) waitEtcdHealthy(ctx context.Context, phase string) error {
	stop := r.startProgress(fmt.Sprintf("waiting for etcd health (%s)", phase))
	defer stop()

	// lastReason is okdctl-authored text safe for Msg; lastErr is the probe
	// error kept for Unwrap, never stringified into Msg.
	var (
		lastReason string
		lastErr    error
	)
	ok := func(ctx context.Context) bool {
		h, err := r.Cluster.EtcdHealthy(ctx)
		if err != nil {
			lastErr = err
			return false
		}
		if !h.Healthy {
			lastReason, lastErr = h.Reason, nil
			return false
		}
		return true
	}
	if err := system.WaitForWithTimeout(ctx, "etcd", phase, ok, r.EtcdGateTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: healthGateMsg("etcd", phase, lastReason), Err: errors.Join(err, lastErr)}
	}
	return nil
}

// healthGateMsg renders a failure message from okdctl-authored text only; the
// probe error (if any) rides ClusterError.Err rather than the message.
func healthGateMsg(subsystem, phase, reason string) string {
	msg := fmt.Sprintf("%s health gate (%s) failed", subsystem, phase)
	if reason != "" {
		msg += ": " + reason
	}
	return msg
}

func (r *Runner) vmTarget(role nodetypes.NodeRole, index int) (node string, vmid int) {
	if r.Cfg.Provider.Proxmox != nil {
		node = r.Cfg.Provider.Proxmox.Node
	}
	return node, nodetypes.VMID(r.Cfg, role, index)
}

// resolveVMID resolves target's node name to vmid/role/Ready via ListNodes, so
// snapshot ops address the right VM without re-deriving the tf index.
func (r *Runner) resolveVMID(ctx context.Context, target string) (vmid int, role nodetypes.NodeRole, ready bool, err error) {
	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return 0, "", false, &errtypes.ClusterError{Msg: msgListNodes, Err: err}
	}
	for _, n := range nodes {
		if n.Name != target {
			continue
		}
		idx, ok := cluster.NodeIndex(n.Name)
		if !ok {
			return 0, "", false, &errtypes.ConfigError{Msg: fmt.Sprintf("cannot derive a terraform index from node name %q", n.Name)}
		}
		_, vmid := r.vmTarget(n.Role, idx)
		return vmid, n.Role, n.Ready, nil
	}
	return 0, "", false, &errtypes.ConfigError{Msg: fmt.Sprintf("node %q not found in cluster; run 'okdctl node list' to list nodes", target)}
}

// powerCycleVM stops→starts the VM so the config-only memory change takes
// effect; the node stays as the current step left it, so errors must not assume
// a cordon.
func (r *Runner) powerCycleVM(ctx context.Context, role nodetypes.NodeRole, index int) error {
	stop := r.startProgress("power-cycling vm to realize the new sizing")
	defer stop()
	if r.Power == nil {
		return &errtypes.ClusterError{Msg: "resize needs Proxmox API access to power-cycle the VM (the config-only memory change does not take effect until a stop→start), but no Proxmox credentials are available"}
	}
	node, vmid := r.vmTarget(role, index)
	if err := r.Power.PowerCycleVM(ctx, node, vmid); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("power-cycle vm %d (resize not realized; re-run to retry)", vmid), Err: err}
	}
	return nil
}

// waitCephHealthy blocks until rook-ceph is healthy (quorum, OSDs up/in, PGs
// active+clean) or times out; a no-op without a rook-ceph toolbox.
func (r *Runner) waitCephHealthy(ctx context.Context, phase string) error {
	stop := r.startProgress("waiting for ceph health (" + phase + ")")
	defer stop()
	var (
		lastReason string
		lastErr    error
	)
	notApplicable := false
	ok := func(ctx context.Context) bool {
		h, err := r.Cluster.CephHealthy(ctx)
		if err != nil {
			lastErr = err
			return false
		}
		if !h.Applicable {
			notApplicable = true
			return true
		}
		if !h.Healthy {
			lastReason, lastErr = h.Reason, nil
			return false
		}
		return true
	}
	if err := system.WaitForWithTimeout(ctx, "ceph", phase, ok, r.CephGateTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: healthGateMsg("ceph", phase, lastReason), Err: errors.Join(err, lastErr)}
	}
	if notApplicable {
		r.Log.Debug("node: rook-ceph not present; ceph gate skipped", "phase", phase)
	}
	return nil
}

// waitNodeReady blocks until node reports Ready; used after a resize apply so
// the next control-plane step never proceeds against a rebooting node.
func (r *Runner) waitNodeReady(ctx context.Context, node string) error {
	stop := r.startProgress(fmt.Sprintf("waiting for node %s to become ready", node))
	defer stop()

	ok := func(ctx context.Context) bool {
		nodes, err := r.Cluster.ListNodes(ctx)
		if err != nil {
			return false
		}
		for _, n := range nodes {
			if n.Name == node {
				return n.Ready
			}
		}
		return false
	}
	if err := system.WaitForWithTimeout(ctx, "node", node+"-ready", ok, r.NodeReadyTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("node %s did not become Ready", node), Err: err}
	}
	return nil
}
