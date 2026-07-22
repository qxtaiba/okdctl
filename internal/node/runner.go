package node

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
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
	VMRunning(ctx context.Context, node string, vmid int) (bool, error)
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
}

// NewRunner wires a Runner with derived work/env directories and default
// timeouts. cfg's terraform environment resolves the env directory.
func NewRunner(cl *cluster.Client, tf *terraform.Executor, cfg *config.Config, projectRoot, configPath, tfEnv, runID string, log *slog.Logger) *Runner {
	workDir := filepath.Join(projectRoot, "okd-install")
	return &Runner{
		Cluster:             cl,
		TF:                  tf,
		Cfg:                 cfg,
		ConfigPath:          configPath,
		ProjectRoot:         projectRoot,
		WorkDir:             workDir,
		EnvDir:              filepath.Join(projectRoot, "infrastructure", "terraform", "environments", tfEnv),
		RunID:               runID,
		Log:                 logutil.OrNop(log),
		Reporter:            logutil.NopProgressReporter,
		NodeReadyTimeout:    DefaultNodeReadyTimeout,
		EtcdGateTimeout:     DefaultEtcdGateTimeout,
		CephGateTimeout:     DefaultCephGateTimeout,
		ClusterReadyTimeout: DefaultClusterReadyTimeout,
	}
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
	for k, v := range planVars {
		vars[k] = v
	}
	return vars
}

// planTargeted runs a single-resource targeted plan with the node-op invariant
// vars plus planVars and gates it to exactly (address, want). On success it
// returns the saved plan's path and a cleanup func the caller MUST invoke once
// done — after any apply, since Apply consumes the saved plan. On any error the
// plan file is already removed and the returned cleanup is a no-op.
func (r *Runner) planTargeted(ctx context.Context, address string, want terraform.PlanAction, planVars map[string]string) (planPath string, cleanup func(), err error) {
	noop := func() {}
	if err := r.TF.Init(ctx); err != nil {
		return "", noop, r.TF.WithLockHint(&errtypes.ClusterError{Msg: "terraform init", Err: err})
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
		return "", noop, r.TF.WithLockHint(&errtypes.ClusterError{Msg: "terraform plan", Err: planErr})
	}

	changes, showErr := r.TF.ShowPlanChanges(ctx, planPath)
	if showErr != nil {
		cleanup()
		return "", noop, &errtypes.ClusterError{Msg: "read terraform plan", Err: showErr}
	}
	if gateErr := terraform.AssertOnlyChange(changes, address, want); gateErr != nil {
		cleanup()
		return "", noop, &errtypes.ClusterError{Msg: "plan safety gate refused the change", Err: gateErr}
	}
	return planPath, cleanup, nil
}

// targetedApply plans a single-resource change, gates the plan to exactly
// (address, want), snapshots state, and applies the saved plan. A gate failure
// aborts before any mutation. planVars are plan-time -var overrides so the
// preview is truthful without persisting terraform.tfvars — the dry-run path
// relies on this to show the intended change while writing nothing to disk.
func (r *Runner) targetedApply(ctx context.Context, address string, want terraform.PlanAction, planVars map[string]string) error {
	stop := r.startProgress(fmt.Sprintf("applying terraform change to %s", address))
	defer stop()

	planPath, cleanup, err := r.planTargeted(ctx, address, want, planVars)
	if err != nil {
		return err
	}
	defer cleanup()

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

// powerCycleVM stops→starts the VM backing a resized node so bpg/proxmox's
// config-only memory change actually takes effect (see PowerCycler). It fails
// closed: without a wired power-cycler the resize cannot be realized, so the
// caller must leave the node cordoned and surface the error.
func (r *Runner) powerCycleVM(ctx context.Context, role nodetypes.NodeRole, index int) error {
	stop := r.startProgress("power-cycling vm to realize the new sizing")
	defer stop()
	if r.Power == nil {
		return &errtypes.ClusterError{Msg: "resize needs Proxmox API access to power-cycle the VM (the config-only memory change does not take effect until a stop→start), but no Proxmox credentials are available"}
	}
	node, vmid := r.vmTarget(role, index)
	if err := r.Power.PowerCycleVM(ctx, node, vmid); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("power-cycle vm %d (node left cordoned; resize not realized)", vmid), Err: err}
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
