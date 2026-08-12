package node

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// ResizeScope selects which nodes a resize targets: all masters, all workers,
// or a single named node (whose role determines the per-role knob mutated).
type ResizeScope struct {
	Role nodetypes.NodeRole
	Node string
}

// ResizeOptions carries the new per-node resources. MemoryMB and CPU are each
// independently optional — 0 keeps the role's current value — but at least
// one must be set.
type ResizeOptions struct {
	MemoryMB int
	CPU      int
	// OSDiskGB grows the role's OS disk (scsi0) to this size. Grow-only:
	// a value at or below the role's current DiskGB is refused. 0 keeps the
	// current size. Realized live via the in-guest grower — no power-cycle —
	// so a disk-only resize never cordons, drains, or restarts the node.
	OSDiskGB int
	// HostTotalMiB / HostAllocatedMiB feed the memory-budget guard when a
	// read-only Proxmox probe supplied them; both zero skips the numeric check.
	HostTotalMiB     int
	HostAllocatedMiB int
	// DatastoreAvailGB feeds the disk-budget guard when the read-only
	// Proxmox probe supplied it; 0 skips the numeric check (warn only).
	DatastoreAvailGB int
	// Acknowledge overrides a stranded marker left by a different op/target so
	// Resize proceeds fresh instead of refusing; see beginOp.
	Acknowledge bool
	// SkipDrain power-cycles the node without cordoning/draining it first. An
	// in-place grow is realized by a hypervisor stop→start that kills the node's
	// pods anyway; skipping the drain lets them restart in place on the roomier
	// node instead of evicting them cluster-wide — which cannot succeed when the
	// cluster is memory-saturated (every other node full → evicted pods wedge
	// Pending → drain times out). The etcd and Ceph health gates still run.
	SkipDrain bool
}

// Resize changes per-role node resources and rolls the change out one node at a
// time. Masters are etcd-health-gated before and after every node and applied
// with an in-place-update plan gate (a replace is refused). Workers roll
// without the etcd gate. Sizing is per-role: the whole role's knob is updated
// in config/tfvars, but each targeted apply mutates only the current node —
// other same-role nodes pick up the pending change on the next full deploy.
// An interrupted role roll resumes at its recorded node/step (see beginOp):
// the node the marker names re-enters at that step, every already-completed
// node in the same role is skipped via a read-only plan probe, and any node
// not yet reached rolls in full.
func (r *Runner) Resize(ctx context.Context, scope ResizeScope, opts ResizeOptions) error {
	if opts.MemoryMB <= 0 && opts.CPU <= 0 && opts.OSDiskGB <= 0 {
		return &errtypes.ConfigError{Msg: "resize: at least one of --memory-mb, --cpu, or --os-disk-gb must be set"}
	}

	if err := validateDiskScope(scope, opts); err != nil {
		return err
	}

	// diskOnly must be known before the power-cycler refusal below (a disk-only
	// resize is realized in-guest, not via power-cycle) and before the
	// opts.MemoryMB = current defaulting further down, so it's computed here,
	// right after the at-least-one gate — it only reads opts.
	diskOnly := opts.OSDiskGB > 0 && opts.MemoryMB <= 0 && opts.CPU <= 0

	// Fail before any disruption: a resize is only realized by whichever
	// collaborator actually applies it, so refuse up front when that
	// collaborator isn't wired rather than draining a node and then
	// discovering the change can't take effect.
	if err := r.requireRealizers(diskOnly, opts); err != nil {
		return err
	}

	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: msgListNodes, Err: err}
	}

	// A dry-run previews a fresh plan and mutates nothing, so resume is
	// irrelevant: skip beginOp entirely so a stranded foreign marker previews
	// rather than refusing.
	var marker *OpMarker
	if !r.DryRun {
		marker, err = r.beginOp(OpResize, resizeScopeMatch(scope, nodes), opts.Acknowledge)
		if err != nil {
			return err
		}
	}
	resuming := marker != nil

	targets, role, err := resolveResizeTargets(nodes, scope)
	if err != nil {
		return &errtypes.ConfigError{Msg: err.Error(), Err: err}
	}

	if opts.OSDiskGB > 0 {
		if cur := r.roleDiskGB(role); opts.OSDiskGB <= cur {
			return &errtypes.ConfigError{Msg: fmt.Sprintf(
				"resize: --os-disk-gb is grow-only: requested %d GiB but %s already have %d GiB", opts.OSDiskGB, role, cur)}
		}
	}

	current := r.roleMemoryMB(role)
	if opts.MemoryMB <= 0 {
		opts.MemoryMB = current // omitted --memory-mb keeps memory unchanged
	}
	// The budget guard runs only on a fresh roll: on a resume the live
	// HostAllocatedMiB probe already counts every target that grew before the
	// crash, so projecting delta*len(targets) on top of it double-counts the
	// finished nodes and can refuse a legitimate resume on a tight host.
	delta := opts.MemoryMB - current
	if !resuming {
		if opts.HostTotalMiB > 0 {
			if err := validateMemoryBudget(opts.HostTotalMiB, opts.HostAllocatedMiB, delta*len(targets)); err != nil {
				return &errtypes.ConfigError{Msg: err.Error(), Err: err}
			}
		} else if delta > 0 {
			r.Log.Warn("node: could not verify host memory budget (no proxmox probe); ensure the host has headroom before growing nodes",
				"delta_mib_per_node", delta, "nodes", len(targets))
		}

		if err := r.checkDatastoreBudget(role, len(targets), opts); err != nil {
			return err
		}
	}

	plan := resizePlan(role, targets, r.Cfg.Cluster.Name, opts)

	// Persist only outside dry-run: a dry-run must write nothing to disk. The
	// truthful plan preview instead comes from sizingVars passed as -var
	// overrides, so terraform sees the new sizing without a tfvars/config write.
	sizingVars := roleSizingVars(role, opts)
	if r.DryRun {
		r.preview(&plan)
	} else if !resuming {
		if err := r.confirmOrDecline(ctx, &plan, "node: resize cancelled", "role", string(role)); err != nil {
			return err
		}
	}

	if !r.DryRun {
		r.Log.Info("node: resizing nodes", "role", string(role), "nodes", len(targets), "memory_mb", opts.MemoryMB)
	}

	for _, t := range targets {
		if err := r.resizeOneNode(ctx, t, role, sizingVars, marker, opts, diskOnly); err != nil {
			return err
		}
	}

	if r.DryRun {
		r.Log.Info("node: dry-run — resize plan gate passed for all targets; no cluster or workspace changes made",
			"role", string(role), "memory_mb", opts.MemoryMB, "nodes", len(targets))
		return nil
	}

	// Persist the role's sizing only once every target has verifiably landed —
	// a crash mid-loop must leave config/tfvars at the pre-resize value so a
	// resumed run recomputes the same target size rather than double-applying a
	// delta on top of an already-persisted one.
	r.applyRoleSizing(role, opts)
	if err := r.persistTopology(); err != nil {
		return &errtypes.ClusterError{Msg: msgPersistTopology, Err: err}
	}

	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}
	r.Log.Info("node: resize complete", "role", string(role), "memory_mb", opts.MemoryMB, "nodes", len(targets))
	return nil
}

// resizePlan builds the informed-confirmation summary for a resize: one
// in-place update per target node, carrying the resolved memory/cpu targets.
func resizePlan(role nodetypes.NodeRole, targets []resizeTarget, clusterName string, opts ResizeOptions) OpPlan {
	nodes := make([]PlanNode, len(targets))
	for i, t := range targets {
		addr := workerAddress(t.index)
		if role == nodetypes.RoleMaster {
			addr = masterAddress(t.index)
		}
		nodes[i] = PlanNode{
			Name:      t.name,
			Role:      role,
			TFAddress: addr,
			Action:    terraform.PlanActionUpdate,
		}
	}
	return OpPlan{
		Op:       OpResize,
		Cluster:  clusterName,
		Nodes:    nodes,
		MemoryMB: opts.MemoryMB,
		CPU:      opts.CPU,
		OSDiskGB: opts.OSDiskGB,
	}
}

// roleSizingVars builds the plan-time -var overrides for a per-role resize so a
// targeted plan reflects the new sizing without persisting terraform.tfvars.
func roleSizingVars(role nodetypes.NodeRole, opts ResizeOptions) map[string]string {
	memKey, cpuKey := "worker_memory_mb", "worker_cpu_cores"
	if role == nodetypes.RoleMaster {
		memKey, cpuKey = "master_memory_mb", "master_cpu_cores"
	}
	vars := map[string]string{memKey: strconv.Itoa(opts.MemoryMB)}
	if opts.CPU > 0 {
		vars[cpuKey] = strconv.Itoa(opts.CPU)
	}
	if opts.OSDiskGB > 0 {
		diskKey := "worker_os_disk_size_gb"
		if role == nodetypes.RoleMaster {
			diskKey = "master_os_disk_size_gb"
		}
		vars[diskKey] = strconv.Itoa(opts.OSDiskGB)
	}
	return vars
}

type resizeTarget struct {
	name  string
	index int
}

func resolveResizeTargets(nodes []cluster.NodeDetail, scope ResizeScope) ([]resizeTarget, nodetypes.NodeRole, error) {
	if scope.Node != "" {
		for _, n := range nodes {
			if n.Name == scope.Node {
				idx, ok := cluster.NodeIndex(n.Name)
				if !ok {
					return nil, "", fmt.Errorf("cannot derive a terraform index from node name %q", n.Name)
				}
				return []resizeTarget{{name: n.Name, index: idx}}, n.Role, nil
			}
		}
		return nil, "", fmt.Errorf("node %q not found in cluster; run 'okdctl node list' to list nodes", scope.Node)
	}

	var targets []resizeTarget
	for _, n := range nodes {
		if n.Role != scope.Role {
			continue
		}
		idx, ok := cluster.NodeIndex(n.Name)
		if !ok {
			return nil, "", fmt.Errorf("cannot derive a terraform index from node name %q", n.Name)
		}
		targets = append(targets, resizeTarget{name: n.Name, index: idx})
	}
	if len(targets) == 0 {
		return nil, "", fmt.Errorf("no %s nodes found to resize", scope.Role)
	}
	slices.SortFunc(targets, func(a, b resizeTarget) int { return cmp.Compare(a.index, b.index) })
	return targets, scope.Role, nil
}

// validateDiskScope refuses a disk grow scoped to a single node. DiskGB
// persists role-wide (applyRoleSizing), and for memory/cpu that's fine — a
// same-role sibling left behind by a single-node resize still picks up the
// pending change on its next full deploy. A disk grow can't: CoreOS only
// runs growpart/xfs_growfs on firstboot, so a sibling's ordinary config-only
// apply would widen its virtual disk at the hypervisor level but never grow
// the in-guest filesystem into it. Worse, a single-node grow persists the
// role's DiskGB immediately, so a later 'resize masters --os-disk-gb <same
// size>' would trip the grow-only check in Resize and refuse — permanently
// locking the siblings out of ever catching up through this command.
// Refusing up front keeps disk sizing role-scoped like the config it
// mutates. Split out of Resize to keep its cyclomatic complexity down.
func validateDiskScope(scope ResizeScope, opts ResizeOptions) error {
	if opts.OSDiskGB <= 0 || scope.Node == "" {
		return nil
	}
	return &errtypes.ConfigError{Msg: fmt.Sprintf(
		"resize: --os-disk-gb is role-scoped and cannot target a single node (%q) — same-role siblings can never catch up on a later full deploy (CoreOS grows the filesystem on firstboot only), so a single-node grow would also permanently block a same-size role-wide resize; use 'masters' or 'workers' instead",
		scope.Node)}
}

// requireRealizers refuses a live resize when the collaborator that would
// realize it isn't wired: a memory/cpu change (or a disk grow bundled with
// one) needs Power to power-cycle the VM, and any disk grow needs Disk to
// realize it in-guest via oc debug. A disk-only resize is exempt from the
// Power requirement — it never power-cycles. A dry-run is exempt from both:
// it only previews, so nothing needs to be realized. Checked once, up front,
// so a resize never drains a node only to discover the change can't land.
func (r *Runner) requireRealizers(diskOnly bool, opts ResizeOptions) error {
	if r.DryRun {
		return nil
	}
	if !diskOnly && r.Power == nil {
		return &errtypes.ConfigError{Msg: "resize needs Proxmox API access to power-cycle the VM after the memory change, but no Proxmox credentials are available; set PROXMOX_VE_* credentials and retry"}
	}
	if opts.OSDiskGB > 0 && r.Disk == nil {
		return &errtypes.ConfigError{Msg: "resize: --os-disk-gb needs cluster access to grow the filesystem in-guest (oc debug), but no disk grower is wired"}
	}
	return nil
}

// checkDatastoreBudget enforces the disk-budget guard for a fresh resize
// roll: numTargets is len(targets), already resolved by the caller. A no-op
// when this resize carries no disk change. Split out of Resize to keep its
// cyclomatic complexity down; see validateDatastoreBudget for the guard's
// thin-provisioning rationale.
func (r *Runner) checkDatastoreBudget(role nodetypes.NodeRole, numTargets int, opts ResizeOptions) error {
	if opts.OSDiskGB <= 0 {
		return nil
	}
	diskDelta := (opts.OSDiskGB - r.roleDiskGB(role)) * numTargets
	if opts.DatastoreAvailGB > 0 {
		if err := validateDatastoreBudget(opts.DatastoreAvailGB, diskDelta); err != nil {
			return &errtypes.ConfigError{Msg: err.Error(), Err: err}
		}
		return nil
	}
	r.Log.Warn("node: could not verify datastore headroom (no proxmox probe); ensure the os datastore fits the grow",
		"delta_gb_total", diskDelta)
	return nil
}

func (r *Runner) roleMemoryMB(role nodetypes.NodeRole) int {
	if role == nodetypes.RoleMaster {
		return r.Cfg.Topology.ControlPlane.MemoryMB
	}
	return r.Cfg.Topology.Workers.MemoryMB
}

func (r *Runner) roleDiskGB(role nodetypes.NodeRole) int {
	if role == nodetypes.RoleMaster {
		return r.Cfg.Topology.ControlPlane.DiskGB
	}
	return r.Cfg.Topology.Workers.DiskGB
}

func (r *Runner) applyRoleSizing(role nodetypes.NodeRole, opts ResizeOptions) {
	nc := &r.Cfg.Topology.Workers
	if role == nodetypes.RoleMaster {
		nc = &r.Cfg.Topology.ControlPlane
	}
	nc.MemoryMB = opts.MemoryMB
	if opts.CPU > 0 {
		nc.CPU = opts.CPU
	}
	if opts.OSDiskGB > 0 {
		nc.DiskGB = opts.OSDiskGB
	}
}

// resizeOneNode rolls one target. When marker is nil the node runs every step
// fresh. When marker names this node, each step is gated via shouldRunStep so
// the node re-enters at the step the interrupted run was about to perform.
// When marker names a different node (another target in the same role roll),
// a read-only plan probe checks whether this node already landed before the
// crash — the resume loop overwrites the marker one node at a time, so an
// earlier target's completion leaves no per-node record of its own — and the
// node is skipped entirely on a positive probe. The probe only runs when a
// marker is present: a fresh roll pays zero extra terraform calls.
func (r *Runner) resizeOneNode(ctx context.Context, t resizeTarget, role nodetypes.NodeRole, sizingVars map[string]string, marker *OpMarker, opts ResizeOptions, diskOnly bool) error {
	isMaster := role == nodetypes.RoleMaster
	address := masterAddress(t.index)
	if !isMaster {
		address = workerAddress(t.index)
	}
	resuming := marker != nil

	// Dry-run performs ZERO cluster mutation (no cordon/drain/uncordon) and
	// ZERO persistence: it only previews the plan gate for the in-place update.
	// Kept ahead of every mutating step so the --dry-run contract holds.
	if r.DryRun {
		return r.targetedApply(ctx, address, terraform.PlanActionUpdate, sizingVars, resuming)
	}

	resumeStep := Step("")
	if marker != nil {
		if marker.Target == t.name {
			resumeStep = marker.Step
		} else {
			_, alreadyAtTarget, cleanup, err := r.planTargeted(ctx, address, terraform.PlanActionUpdate, sizingVars, resuming)
			cleanup()
			if err != nil {
				return err
			}
			if alreadyAtTarget {
				r.Log.Info("node: resize target already at desired size — skipping", "node", t.name, "role", string(role))
				return nil
			}
		}
	}

	// The pre-gate is skipped only when resuming exactly at StepPowerCycle:
	// PowerCycleVM is stop-then-start, so a crash (or a transient start
	// failure) between the two may leave this master powered OFF — etcd
	// cannot report healthy until the very power-on this gate would forever
	// wait ahead of. Every other resume point — before the power-cycle was
	// ever attempted, or at/after StepDiskGrow/StepUncordon, where a
	// power-cycle (if this resize even has one) already completed in the
	// crashed run — finds the node already up, so the pre-gate is safe, and
	// required, to run. This must NOT widen to "at or past power-cycle": that
	// wider check also caught StepDiskGrow (a disk-only resize never
	// power-cycles at all, so the node was never down) and silently skipped
	// the gate for it too.
	skipPreGate := resumeStep == StepPowerCycle
	if isMaster && !skipPreGate {
		if err := r.waitEtcdHealthy(ctx, "pre-"+t.name); err != nil {
			return err
		}
	}

	// Control-plane nodes host standalone guard pods (etcd-guard,
	// kube-apiserver-guard, …) that declare no controller; oc adm drain refuses
	// those without --force, so a master resize passes force=isMaster.
	//
	// --skip-drain power-cycles without cordon/drain: the stop→start kills the
	// pods anyway and they restart in place on the roomier node, avoiding the
	// eviction storm that deadlocks a drain on a memory-saturated cluster. The
	// etcd/Ceph gates below still bound the reboot's blast radius. A disk-only
	// resize is realized in-guest with no reboot at all, so it skips
	// cordon/drain outright — there is no eviction to avoid in the first place.
	if !diskOnly && !opts.SkipDrain {
		if err := r.cordonAndDrain(ctx, OpResize, t.name, defaultDrainTimeout, isMaster, resumeStep); err != nil {
			return err
		}
	}

	if err := r.runStep(OpResize, t.name, StepTFApply, resumeStep, func() error {
		// A memory change must be an in-place update, never a replace — a
		// replace would destroy the VM and, for a master, break quorum.
		// prevent_destroy on the master resource backstops this, but the gate
		// refuses it up front with a clear message instead of a terraform
		// apply error.
		return r.targetedApply(ctx, address, terraform.PlanActionUpdate, sizingVars, resuming)
	}); err != nil {
		return err
	}

	// The apply only rewrites the VM's *config*; bpg/proxmox does not restart it,
	// so the guest keeps its old memory until a hypervisor stop→start. Power-cycle
	// now, then wait for the node to rejoin — unless disk-only, which needs no
	// power-cycle at all: the grow is realized in-guest below without ever
	// taking the node down. A failure here leaves the node cordoned and returns
	// an error — never report success on an unrealized resize.
	if err := r.realizePowerCycle(ctx, t, role, resumeStep, diskOnly); err != nil {
		return err
	}

	// A disk grow (disk-only or bundled with a memory/cpu change) is realized
	// in-guest via oc debug — SCSI rescan → growpart → xfs_growfs — since the
	// terraform apply above only widens the virtual disk at the hypervisor
	// level; the guest filesystem never sees it without this step.
	if err := r.growNodeDisk(ctx, t, opts, resumeStep); err != nil {
		return err
	}

	// Unlike the pre-gate, the post-gate is NOT conditioned on the resume
	// point: by this line, whatever this resize actually does to the node —
	// realizePowerCycle's power-cycle+waitNodeReady for a memory/cpu change,
	// or growNodeDisk's in-guest grow for a disk change — has already
	// completed in THIS run, so the node is expected to be up regardless of
	// where an earlier crash left the marker. Quorum must be re-verified here
	// before the node returns to service — never on the strength of a gate
	// that ran before the disruption.
	if isMaster {
		if err := r.waitEtcdHealthy(ctx, "post-"+t.name); err != nil {
			return err
		}
	}

	// A disk-only resize never cordoned, so there is nothing to uncordon and
	// no reboot-driven OSD rebalance to wait out.
	if !diskOnly {
		if err := r.runStep(OpResize, t.name, StepUncordon, resumeStep, func() error {
			if err := r.Cluster.Uncordon(ctx, t.name); err != nil {
				return err
			}

			// A worker (or a compacted master) may host OSDs; the power-cycle took
			// them down and triggered a rebalance. Wait for structural Ceph health
			// before the op returns so a compact loop never drains the next node
			// mid-recovery.
			return r.waitCephHealthy(ctx, "post-"+t.name)
		}); err != nil {
			return err
		}
	}

	r.Log.Info("node: resized", "node", t.name, "role", string(role))
	return nil
}

// realizePowerCycle stops→starts the VM and waits for the node to rejoin, so
// a config-only memory/cpu change (bpg/proxmox never restarts the guest)
// actually takes effect. A disk-only resize skips this entirely: the grow is
// realized in-guest by growNodeDisk below without ever taking the node down.
func (r *Runner) realizePowerCycle(ctx context.Context, t resizeTarget, role nodetypes.NodeRole, resumeStep Step, diskOnly bool) error {
	if diskOnly {
		return nil
	}
	if err := r.runStep(OpResize, t.name, StepPowerCycle, resumeStep, func() error {
		return r.powerCycleVM(ctx, role, t.index)
	}); err != nil {
		return err
	}
	return r.waitNodeReady(ctx, t.name)
}

// growNodeDisk realizes a pending OS-disk grow in-guest via oc debug (SCSI
// rescan → growpart → xfs_growfs). A no-op when this resize carries no disk
// change, or when resumeStep is already past this step.
func (r *Runner) growNodeDisk(ctx context.Context, t resizeTarget, opts ResizeOptions, resumeStep Step) error {
	if opts.OSDiskGB <= 0 {
		return nil
	}
	return r.runStep(OpResize, t.name, StepDiskGrow, resumeStep, func() error {
		stop := r.startProgress(fmt.Sprintf("growing os disk on %s", t.name))
		defer stop()
		if err := r.Disk.GrowOSDisk(ctx, t.name); err != nil {
			return fmt.Errorf("grow os disk on %s: %w", t.name, err)
		}
		return nil
	})
}
