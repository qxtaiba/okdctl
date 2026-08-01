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
	// HostTotalMiB / HostAllocatedMiB feed the memory-budget guard when a
	// read-only Proxmox probe supplied them; both zero skips the numeric check.
	HostTotalMiB     int
	HostAllocatedMiB int
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
	if opts.MemoryMB <= 0 && opts.CPU <= 0 {
		return &errtypes.ConfigError{Msg: "resize: at least one of --memory-mb or --cpu must be set"}
	}

	// Fail before any disruption: a resize is realized only by a hypervisor
	// power-cycle, so refuse up front when no power-cycler is wired rather than
	// draining a node and then discovering the change can't take effect.
	if !r.DryRun && r.Power == nil {
		return &errtypes.ConfigError{Msg: "resize needs Proxmox API access to power-cycle the VM after the memory change, but no Proxmox credentials are available; set PROXMOX_VE_* credentials and retry"}
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
		if err := r.resizeOneNode(ctx, t, role, sizingVars, marker, opts.SkipDrain); err != nil {
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

func (r *Runner) roleMemoryMB(role nodetypes.NodeRole) int {
	if role == nodetypes.RoleMaster {
		return r.Cfg.Topology.ControlPlane.MemoryMB
	}
	return r.Cfg.Topology.Workers.MemoryMB
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
func (r *Runner) resizeOneNode(ctx context.Context, t resizeTarget, role nodetypes.NodeRole, sizingVars map[string]string, marker *OpMarker, skipDrain bool) error {
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

	// The pre-gate is skipped when the resume point is at or past the
	// power-cycle: PowerCycleVM is stop-then-start, so a crash (or a transient
	// start failure) between the two leaves this master powered OFF — etcd
	// cannot report healthy until the very power-on this gate would forever
	// wait ahead of. The post-cycle gate below still verifies quorum before
	// the node returns to service.
	preGate := resumeStep == "" || stepOrder[resumeStep] < stepOrder[StepPowerCycle]
	if isMaster && preGate {
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
	// etcd/Ceph gates below still bound the reboot's blast radius.
	if !skipDrain {
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
	// now, then wait for the node to rejoin. A failure here leaves the node
	// cordoned and returns an error — never report success on an unrealized resize.
	if err := r.runStep(OpResize, t.name, StepPowerCycle, resumeStep, func() error {
		return r.powerCycleVM(ctx, role, t.index)
	}); err != nil {
		return err
	}

	if err := r.waitNodeReady(ctx, t.name); err != nil {
		return err
	}
	// The post-cycle gate always runs for masters, including on a resume at or
	// past the power-cycle: the pre-gate skip above only avoids deadlocking
	// behind a power-on, but once the node is Ready again quorum must be
	// verified before it returns to service.
	if isMaster {
		if err := r.waitEtcdHealthy(ctx, "post-"+t.name); err != nil {
			return err
		}
	}

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

	r.Log.Info("node: resized", "node", t.name, "role", string(role))
	return nil
}
