package node

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// Default gates for node-lifecycle waits. NodeReadyTimeout bounds the wait for
// a resized/rejoined node to report Ready; EtcdGateTimeout bounds the pre/post
// etcd-quorum health gate around a control-plane mutation.
const (
	DefaultNodeReadyTimeout = 15 * time.Minute
	DefaultEtcdGateTimeout  = 10 * time.Minute
	// DefaultCephGateTimeout is generous: a rebalance after an OSD host is
	// power-cycled or removed can take a while to return all PGs to active+clean.
	DefaultCephGateTimeout = 30 * time.Minute
	// DefaultClusterReadyTimeout bounds cluster start's wait for every node to
	// report Ready, ticking ApprovePendingCSRs each poll so kubelet certificate
	// rotation never stalls the wait unattended.
	DefaultClusterReadyTimeout = 30 * time.Minute
	// DefaultSnapshotTaskTimeout bounds how long a snapshot create/rollback/
	// delete waits for its async pvesh task to reach status=stopped.
	DefaultSnapshotTaskTimeout = 5 * time.Minute
	// hostMemoryReserveMiB is the hypervisor headroom the memory-budget guard
	// keeps free when projecting a resize (ZFS ARC, host services).
	hostMemoryReserveMiB = 2048
	// vmidMasterOffset / vmidWorkerOffset mirror the module's numbering
	// (bootstrap=base, masters=base+10+n, workers=base+100+n) so a resize can
	// address the right QEMU id for the Proxmox power-cycle.
	vmidMasterOffset = 10
	vmidWorkerOffset = 100
)

// clusterClient is the slice of cluster.Client the node ops drive. Defined as
// an interface so tests can substitute a call-recording fake (proving, e.g.,
// that a dry-run performs no cordon/drain) without a live cluster.
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

// vmPowerCycler stops→starts a VM through the Proxmox API. An interface so a
// resize test can record the call without a live hypervisor. nil means no
// Proxmox credentials were wired — resize then refuses rather than silently
// leaving the memory change unrealized.
type vmPowerCycler interface {
	PowerCycleVM(ctx context.Context, node string, vmid int) error
	ShutdownVM(ctx context.Context, node string, vmid int) error
	StartVM(ctx context.Context, node string, vmid int) error
}

// snapshotClient mirrors package hostssh's pvesh-backed snapshot primitives
// as an interface so a test can substitute a call-recording fake without a
// live Proxmox host, the same role vmPowerCycler plays for the REST API.
type snapshotClient interface {
	CreateSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name, description string, timeout time.Duration) error
	ListSnapshots(ctx context.Context, p *hostssh.RemoteISOParams, vmid int) ([]hostssh.SnapshotInfo, error)
	RollbackSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name string, timeout time.Duration) error
	DeleteSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name string, timeout time.Duration) error
	VMAgentEnabled(ctx context.Context, p *hostssh.RemoteISOParams, vmid int) (bool, error)
}

// HostsshSnapshotClient is the production snapshotClient, delegating each
// call straight through to package hostssh's pvesh primitives.
type HostsshSnapshotClient struct{}

// CreateSnapshot forwards to hostssh.CreateSnapshot.
func (HostsshSnapshotClient) CreateSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name, description string, timeout time.Duration) error {
	return hostssh.CreateSnapshot(ctx, p, vmid, name, description, timeout)
}

// ListSnapshots forwards to hostssh.ListSnapshots.
func (HostsshSnapshotClient) ListSnapshots(ctx context.Context, p *hostssh.RemoteISOParams, vmid int) ([]hostssh.SnapshotInfo, error) {
	return hostssh.ListSnapshots(ctx, p, vmid)
}

// RollbackSnapshot forwards to hostssh.RollbackSnapshot.
func (HostsshSnapshotClient) RollbackSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name string, timeout time.Duration) error {
	return hostssh.RollbackSnapshot(ctx, p, vmid, name, timeout)
}

// DeleteSnapshot forwards to hostssh.DeleteSnapshot.
func (HostsshSnapshotClient) DeleteSnapshot(ctx context.Context, p *hostssh.RemoteISOParams, vmid int, name string, timeout time.Duration) error {
	return hostssh.DeleteSnapshot(ctx, p, vmid, name, timeout)
}

// VMAgentEnabled forwards to hostssh.VMAgentEnabled.
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

// isoProvisioner is the slice of setup.Phase node add drives to build and
// upload the new node's custom CoreOS ISO. Both calls are already
// count-generic and fingerprint/checksum-skip unchanged nodes, so node add
// reuses them as-is rather than a dedicated single-node build path. An
// interface so a test can substitute a call-recording fake without shelling
// out to coreos-installer/scp.
type isoProvisioner interface {
	BuildCustomISOs(ctx context.Context, cfg *config.Config, opts *setup.Options) error
	UploadCustomISOsToProxmox(ctx context.Context, cfg *config.Config, opts *setup.Options) error
}

// ignitionServer is the slice of setup.Phase node add drives to expose
// worker.ign over HTTPS for the join window. An interface so a test can
// substitute a call-recording fake without touching httpd. A teardown error
// means httpd may still be serving the pull secret; the caller must surface
// it loudly rather than swallow it.
type ignitionServer interface {
	ReviveIgnitionServer(ctx context.Context, cfg *config.Config, projectRoot, clusterDir string) error
	TeardownIgnitionServer(ctx context.Context) error
}

// Runner drives node-lifecycle ops against one cluster. TF mutates VMs;
// Cluster runs the Kubernetes lifecycle. ConfigPath and EnvDir are persisted
// on each op so a later full deploy reconciles to the same topology.
type Runner struct {
	Cluster     clusterClient
	TF          terraformExec
	Cfg         *config.Config
	ConfigPath  string
	ProjectRoot string
	WorkDir     string
	EnvDir      string
	RunID       string
	DryRun      bool
	Log         *slog.Logger
	Reporter    logutil.ProgressReporter

	// Power performs the post-resize hypervisor power-cycle. nil when no
	// Proxmox credentials are available; a resize then fails safe.
	Power vmPowerCycler

	// Proxmox carries the pvesh-over-SSH connection params snapshot ops use.
	// nil when no Proxmox SSH access is wired — snapshot ops then fail closed
	// the same way Power does for resize.
	Proxmox *hostssh.RemoteISOParams

	// Snapshot is the pvesh-backed client snapshot ops drive; an interface so
	// tests can record calls without a live hypervisor.
	Snapshot snapshotClient

	// SnapshotTaskTimeout bounds how long a snapshot create/rollback/delete
	// waits for its async pvesh task to complete.
	SnapshotTaskTimeout time.Duration

	// ISO and Ignition drive node add's ISO build/upload and ignition-server
	// revive (node add only; nil for every other op). SetupOpts carries the
	// setup phase's WorkDir/ProjectRoot/TerraformEnv those calls need.
	ISO       isoProvisioner
	Ignition  ignitionServer
	SetupOpts *setup.Options

	// Confirm gates each mutating op between guards/preflight and the first
	// mutation; nil auto-approves (tests, non-interactive callers that gate
	// elsewhere). Preview renders the dry-run plan; nil falls back to slog.
	Confirm ConfirmFunc
	Preview PreviewFunc

	// preConsented suppresses the confirm gate for ops composed under a consent
	// already granted at a higher level (compact's RemoveWorker/Resize inner
	// calls). Set for the duration of the composed sequence and restored after;
	// see Compact.
	preConsented bool

	NodeReadyTimeout    time.Duration
	EtcdGateTimeout     time.Duration
	CephGateTimeout     time.Duration
	ClusterReadyTimeout time.Duration

	// tfEnv is the terraform environment name captured by WithTerraformEnv;
	// NewRunner resolves it against ProjectRoot into EnvDir after all
	// options have applied, so option ordering does not matter.
	tfEnv string
}

// RunnerOption configures optional Runner wiring in NewRunner, mirroring the
// okd.New / addon.NewManager option style.
type RunnerOption func(*Runner)

// WithProjectRoot sets the project root the work and terraform env
// directories derive from.
func WithProjectRoot(root string) RunnerOption {
	return func(r *Runner) { r.ProjectRoot = root }
}

// WithConfigPath sets the okdctl.yaml path each op persists topology to.
func WithConfigPath(path string) RunnerOption {
	return func(r *Runner) { r.ConfigPath = path }
}

// WithTerraformEnv selects the terraform environment whose directory node
// ops plan and apply in.
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

// NewRunner wires a Runner with derived work/env directories and default
// timeouts. Required collaborators (cluster client, terraform executor,
// config) are positional; everything else arrives via options.
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
	r.WorkDir = filepath.Join(r.ProjectRoot, system.WorkDirName)
	r.EnvDir = system.TerraformEnvDir(r.ProjectRoot, r.tfEnv)
	return r
}

func (r *Runner) marker() string { return markerPath(r.WorkDir) }

// startProgress starts r.Reporter for desc, unless dry-run — a dry-run must
// stay visually silent even for gates that run ahead of the dry-run branch
// (e.g. compact's pre-flight etcd check), so the check lives here once
// rather than at every call site. Reporter is nil-safe: Runner values built
// directly (tests) skip NewRunner's NopProgressReporter default.
func (r *Runner) startProgress(desc string) (stop func()) {
	if r.DryRun {
		return func() {}
	}
	if r.Reporter == nil {
		return logutil.NopProgressReporter(desc)
	}
	return r.Reporter(desc)
}

// persistTopology re-renders terraform.tfvars from the mutated config and saves
// okdctl.yaml, so the reduced/resized topology survives into the next full
// deploy. WriteTerraformVars preserves the bootstrap sentinel (unlike deploy's
// GenerateTerraformVars) so a re-render cannot resurrect the bootstrap VM.
func (r *Runner) persistTopology() error {
	if err := setup.WriteTerraformVars(r.Cfg, r.EnvDir); err != nil {
		return fmt.Errorf("render terraform vars: %w", err)
	}
	if err := config.NewLoader().Save(r.Cfg, r.ConfigPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// nodeOpPlanVars merges the caller's plan-time overrides onto the post-deploy
// invariants every node op asserts: the bootstrap VM is gone and workers run.
// Passed as -var (highest precedence, above terraform.tfvars and auto-tfvars) so
// a stale terraform.tfvars bootstrap_enabled=true can't inject a bootstrap-create
// into the targeted plan, and the module's start_workers_immediately=false
// default can't plan a running worker to stopped. Either would trip the
// single-change gate or, on apply, stop a healthy VM. planVars override these on
// a key collision.
func nodeOpPlanVars(planVars map[string]string) map[string]string {
	vars := map[string]string{
		"bootstrap_enabled":         "false",
		"start_workers_immediately": "true",
	}
	maps.Copy(vars, planVars)
	return vars
}

// planTargeted runs a single-resource targeted plan with the node-op invariant
// vars plus planVars and gates it to exactly (address, want). On success it
// returns the saved plan's path and a cleanup func the caller MUST invoke once
// done — after any apply, since Apply consumes the saved plan. On any error the
// plan file is already removed and the returned cleanup is a no-op.
//
// An empty plan normally means the gate refused the change (the variable
// never reached the module). But an empty plan is also what a resumed re-run
// produces once the apply already landed, so an empty plan additionally
// checks state via StateHasResource and classifies it with
// terraform.EmptyPlanMeansAlreadyAtTarget. That extra state read only happens
// on the empty-plan path, never on the happy path of a single matching change.
//
// A delete is classified unconditionally: an empty delete plan with the
// address absent from state is unambiguously "already gone". An update is
// classified as already-at-target ONLY when resuming: on a fresh run an empty
// update plan means the variable never reached the module (a -var plumbing
// regression), which must stay fatal rather than be silently reported as
// "already resized".
func (r *Runner) planTargeted(ctx context.Context, address string, want terraform.PlanAction, planVars map[string]string, resuming bool) (planPath string, alreadyAtTarget bool, cleanup func(), err error) {
	noop := func() {}
	if err := r.TF.Init(ctx); err != nil {
		return "", false, noop, r.TF.WithLockHint(&errtypes.ClusterError{Msg: "terraform init", Err: err})
	}

	planFile := "node-op.tfplan"
	planPath = filepath.Join(r.EnvDir, planFile)
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

// targetedApply plans a single-resource change, gates the plan to exactly
// (address, want), snapshots state, and applies the saved plan. A gate failure
// aborts before any mutation. planVars are plan-time -var overrides so the
// preview is truthful without persisting terraform.tfvars — the dry-run path
// relies on this to show the intended change while writing nothing to disk.
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
			// A fresh delete landing here means terraform state has no record
			// of the resource. That is the expected shape after an
			// acknowledged partial remove, but it is also exactly what a
			// lost/mismatched state file produces — in which case the VM still
			// exists and reporting a quiet success would strand it unmanaged.
			r.Log.Warn("node: terraform state has no record of this resource — treating it as already destroyed; if the vm still exists in proxmox, the workspace state is wrong and this result must not be trusted",
				"address", address)
		} else {
			r.Log.Info("node: already at target — skipping apply", "address", address, "action", string(want))
		}
		return nil
	}

	if r.DryRun {
		r.Log.Info("node: dry-run — plan gate passed, skipping apply", "address", address, "action", string(want))
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
// out. It is the mandatory pre/post gate around any control-plane mutation.
func (r *Runner) waitEtcdHealthy(ctx context.Context, phase string) error {
	stop := r.startProgress(fmt.Sprintf("waiting for etcd health (%s)", phase))
	defer stop()

	var lastReason string
	ok := func(ctx context.Context) bool {
		h, err := r.Cluster.EtcdHealthy(ctx)
		if err != nil {
			lastReason = err.Error()
			return false
		}
		if !h.Healthy {
			lastReason = h.Reason
			return false
		}
		return true
	}
	if err := system.WaitForWithTimeout(ctx, "etcd", phase, ok, r.EtcdGateTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("etcd health gate (%s) failed: %s", phase, lastReason), Err: err}
	}
	return nil
}

// vmTarget resolves the Proxmox node name and QEMU vmid for a role/index pair,
// mirroring the module's numbering (bootstrap=base, masters=base+10+n,
// workers=base+100+n) so power-cycle/shutdown/start calls address the right VM.
func (r *Runner) vmTarget(role nodetypes.NodeRole, index int) (node string, vmid int) {
	base := r.Cfg.Topology.VMIDBase
	if base == 0 {
		base = config.DefaultVMIDBase
	}
	offset := vmidWorkerOffset
	if role == nodetypes.RoleMaster {
		offset = vmidMasterOffset
	}
	vmid = base + offset + index
	if r.Cfg.Provider.Proxmox != nil {
		node = r.Cfg.Provider.Proxmox.Node
	}
	return node, vmid
}

// resolveVMID resolves target's cluster node name to its Proxmox vmid, role,
// and current Ready status via ListNodes, so snapshot ops can address the
// right VM without callers re-deriving the terraform index themselves.
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

// powerCycleVM stops→starts the VM backing a resized node so bpg/proxmox's
// config-only memory change actually takes effect (see PowerCycler). It fails
// closed: without a wired power-cycler the resize cannot be realized, so the
// caller surfaces the error and re-runs to retry. The node is left as the
// current step found it — cordoned on the drain path, untouched under
// --skip-drain — so the message must not assume a cordon.
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

// waitCephHealthy blocks until rook-ceph is structurally healthy (mons in
// quorum, OSDs up/in, PGs active+clean) or the gate times out. Clusters without
// a rook-ceph toolbox are treated as not-applicable and pass immediately, so
// the gate is a no-op on non-Ceph clusters.
func (r *Runner) waitCephHealthy(ctx context.Context, phase string) error {
	stop := r.startProgress("waiting for ceph health (" + phase + ")")
	defer stop()
	var lastReason string
	notApplicable := false
	ok := func(ctx context.Context) bool {
		h, err := r.Cluster.CephHealthy(ctx)
		if err != nil {
			lastReason = err.Error()
			return false
		}
		if !h.Applicable {
			notApplicable = true
			return true
		}
		if !h.Healthy {
			lastReason = h.Reason
			return false
		}
		return true
	}
	if err := system.WaitForWithTimeout(ctx, "ceph", phase, ok, r.CephGateTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("ceph health gate (%s) failed: %s", phase, lastReason), Err: err}
	}
	if notApplicable {
		r.Log.Debug("node: rook-ceph not present; ceph gate skipped", "phase", phase)
	}
	return nil
}

// waitNodeReady blocks until node reports Ready. Used after a resize apply so
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
