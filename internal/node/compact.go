package node

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// CompactOptions tunes cluster compaction.
type CompactOptions struct {
	// IngressReplicas sets the compact IngressController replica count (masters
	// serve ingress once workers are gone).
	IngressReplicas int
	// GrowMasterMemoryMB, when > 0, resizes each master after the workers are
	// removed, interleaved so growth never precedes freeing a worker.
	GrowMasterMemoryMB int
	ForceStorage       bool
	// Host memory budget, from a read-only Proxmox probe; zero skips the check.
	HostTotalMiB     int
	HostAllocatedMiB int
	// Acknowledge overrides a stranded marker left by a different op/target,
	// threaded into every inner RemoveWorker/Resize call; see beginOp. A marker
	// from an op compact does NOT compose (anything but OpRemove/OpResize) is
	// refused up front, before any control-plane mutation. An OpRemove/OpResize
	// marker is left to the inner op's own beginOp resume path — it is
	// indistinguishable from compact's own in-flight inner op (one cluster per
	// workdir), so refusing it would break compact's own resume.
	Acknowledge bool
}

// compactVerdict is one worker's read-only preflight result. osds/ingress are
// the pods on the worker (informational; ingress is remediated by compact
// itself), blocked is a storage-guard or plan-gate refusal (nil = clear).
type compactVerdict struct {
	node    string
	index   int
	osds    []string
	ingress []string
	blocked error
}

// compactPreflight aggregates the read-only preflight across every worker plus
// the interleaved-grow memory projection. blockErr is the first refusal (a
// worker guard or the memory budget); nil means the whole plan is clear.
type compactPreflight struct {
	verdicts []compactVerdict
	memErr   error
	blockErr error
}

// Compact consolidates the cluster onto its control plane: make masters
// schedulable, apply the compact IngressController, then remove workers
// top-down — interleaving an optional master grow after each removal so the
// memory budget is respected (a freed worker precedes a grown master). It
// composes RemoveWorker and Resize; it adds no new mutation mechanics.
//
// Every worker guard (rook-ceph OSD presence, per-worker delete plan gate) and
// the memory-budget projection run in a read-only preflight BEFORE the control
// plane is made schedulable — a refusal on the third worker must not leave the
// cluster half-mutated. --dry-run runs the same preflight and prints the ordered
// action list with per-node verdicts, mutating nothing.
//
// The pre-flight etcd gate blocks (up to EtcdGateTimeout) only on the real
// path. Under --dry-run it degrades to a single non-blocking probe reported as
// a verdict line, so a preview against a degraded quorum still prints instead
// of hanging on a gate whose whole purpose is to wait.
//
// Compact records no op marker of its own: RemoveWorker and Resize each
// record OpRemove/OpResize against the node they are mutating, so a crash
// mid-loop leaves a marker naming that node, not compact. A re-run repeats
// the read-only preflight and the idempotent schedulable/ingress step, then
// reaches the same worker (or master) whose own beginOp resumes it at its
// recorded step. The control-plane prep (SetMastersSchedulable + compact
// ingress) is idempotent and reversible, so running it ahead of the inner
// resume is safe; a marker from an op compact does not compose is refused
// before that prep (see refuseForeignMarkerBeforeCompact).
func (r *Runner) Compact(ctx context.Context, opts CompactOptions) error {
	if !r.DryRun {
		if err := r.refuseForeignMarkerBeforeCompact(opts.Acknowledge); err != nil {
			return err
		}
		if err := r.waitEtcdHealthy(ctx, "compact-preflight"); err != nil {
			return err
		}
	}

	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: msgListNodes, Err: err}
	}
	workers := workersByIndexDesc(nodes, r.Log)
	masters := mastersByIndexAsc(nodes, r.Log)

	pf, err := r.runPreflightCompact(ctx, workers, masters, opts)
	if err != nil {
		return err
	}

	plan := compactPlan(pf, r.Cfg.Cluster.Name, opts)

	if r.DryRun {
		r.preview(&plan)
		r.reportCompactPlan(ctx, workers, masters, pf, opts)
		return pf.blockErr
	}
	if pf.blockErr != nil {
		return pf.blockErr
	}

	proceed, err := r.confirm(ctx, &plan)
	if err != nil {
		return err
	}
	if !proceed {
		r.Log.Info("node: compact cancelled", "cluster", r.Cfg.Cluster.Name)
		return ErrDeclined
	}

	// The inner RemoveWorker/Resize calls run under the consent granted above;
	// suppress their gates so no mid-teardown prompt can abort a half-executed
	// sequence and no inner decline can be conflated with success.
	r.preConsented = true
	defer func() { r.preConsented = false }()

	// Preflight passed and confirmed: only now mutate the control plane.
	if err := r.enableSchedulableAndIngress(ctx, opts.IngressReplicas); err != nil {
		return err
	}

	allocated := opts.HostAllocatedMiB
	workerMem := r.Cfg.Topology.Workers.MemoryMB
	masterGrows := 0
	for i, w := range workers {
		if err := r.RemoveWorker(ctx, w, RemoveOptions{ForceStorage: opts.ForceStorage, DrainTimeout: defaultDrainTimeout, Acknowledge: opts.Acknowledge}); err != nil {
			return r.compactHybridError(i, len(workers), w, err)
		}
		if opts.HostAllocatedMiB > 0 {
			allocated -= workerMem
		}
		// Interleave: after freeing a worker, grow the next master so allocation
		// never peaks above the pre-compaction commitment; the freed worker's
		// memory is discounted from the budget passed to the grow.
		if opts.GrowMasterMemoryMB > 0 && masterGrows < len(masters) && i < len(masters) {
			if err := r.growMaster(ctx, masters[masterGrows], allocated, opts); err != nil {
				return err
			}
			masterGrows++
		}
	}

	// Grow any remaining masters once all workers are gone.
	for ; opts.GrowMasterMemoryMB > 0 && masterGrows < len(masters); masterGrows++ {
		if err := r.growMaster(ctx, masters[masterGrows], allocated, opts); err != nil {
			return err
		}
	}

	if err := r.waitEtcdHealthy(ctx, "compact-final"); err != nil {
		return err
	}
	if err := r.waitCephHealthy(ctx, "compact-final"); err != nil {
		return err
	}
	r.Log.Info("node: compaction complete", "masters", len(masters))
	return nil
}

// runPreflightCompact wraps preflightCompact in one reporter span covering
// the whole N-worker plan-gate sequence, rather than a spinner flickering
// per worker.
func (r *Runner) runPreflightCompact(ctx context.Context, workers, masters []string, opts CompactOptions) (compactPreflight, error) {
	stop := r.startProgress(fmt.Sprintf("gating removal plan for %d worker node(s)", len(workers)))
	defer stop()
	return r.preflightCompact(ctx, workers, masters, opts)
}

// preflightCompact runs the read-only guards for every worker (storage +
// per-worker delete plan gate) and the interleaved-grow memory projection,
// mutating nothing. Storage is the guard compact does NOT resolve; ingress is
// remediated by compact itself (masters made schedulable + compact
// IngressController) so it is reported per worker, not blocked here.
func (r *Runner) preflightCompact(ctx context.Context, workers, masters []string, opts CompactOptions) (compactPreflight, error) {
	osdPods, err := r.Cluster.PodsForSelector(ctx, "", "app=rook-ceph-osd")
	if err != nil {
		return compactPreflight{}, &errtypes.ClusterError{Msg: "storage guard: list rook-ceph osd pods", Err: err}
	}
	routerPods, err := r.Cluster.PodsForSelector(ctx, "openshift-ingress", "")
	if err != nil {
		return compactPreflight{}, &errtypes.ClusterError{Msg: "ingress guard: list router pods", Err: err}
	}

	pf := compactPreflight{verdicts: make([]compactVerdict, 0, len(workers))}
	for _, w := range workers {
		idx, _ := cluster.NodeIndex(w)
		v := compactVerdict{
			node:    w,
			index:   idx,
			osds:    podNamesOnNode(osdPods, w),
			ingress: podNamesOnNode(routerPods, w),
		}
		v.blocked = storageGuardVerdict(w, v.osds, opts.ForceStorage, r.Log)
		if v.blocked == nil {
			v.blocked = r.assertWorkerDeletable(ctx, idx)
		}
		if v.blocked != nil && pf.blockErr == nil {
			pf.blockErr = v.blocked
		}
		pf.verdicts = append(pf.verdicts, v)
	}

	pf.memErr = r.projectCompactMemory(len(workers), len(masters), opts)
	if pf.memErr != nil && pf.blockErr == nil {
		pf.blockErr = pf.memErr
	}
	return pf, nil
}

// refuseForeignMarkerBeforeCompact refuses (unless ack) when an on-disk marker
// records an op compact does not compose. Compact drives OpRemove and OpResize
// through its inner calls, and a stranded marker for either is indistinguishable
// from compact's own in-flight inner op (one cluster per workdir) — refusing it
// would break compact's own resume, so those are left to the inner beginOp. A
// marker from any other op family is unambiguously foreign and refused here,
// ahead of the idempotent control-plane prep, so a genuinely-unrelated op is
// never overwritten without an explicit acknowledgement.
func (r *Runner) refuseForeignMarkerBeforeCompact(ack bool) error {
	return r.refuseForeignMarker(ack, OpRemove, OpResize)
}

// assertWorkerDeletable plan-gates the delete of worker[idx] without applying:
// worker[idx] leaves the config when worker_count drops to idx, so the plan-time
// -var override drives the count down exactly as the real removal loop will,
// one step per prior removal. The saved plan is dropped immediately (gate-only).
func (r *Runner) assertWorkerDeletable(ctx context.Context, idx int) error {
	countVars := map[string]string{tfVarWorkerCount: strconv.Itoa(idx)}
	_, _, cleanup, err := r.planTargeted(ctx, workerAddress(idx), terraform.PlanActionDelete, countVars, false)
	cleanup()
	return err
}

// projectCompactMemory refuses the plan when the interleaved master grows would
// exceed the host memory budget. It projects the peak allocation reached across
// the whole remove-then-grow sequence (a freed worker precedes each grow). No
// grow or no probe is not an error — a missing probe degrades to a warning.
func (r *Runner) projectCompactMemory(numWorkers, numMasters int, opts CompactOptions) error {
	if opts.GrowMasterMemoryMB <= 0 {
		return nil
	}
	if opts.HostTotalMiB <= 0 {
		r.Log.Warn("node: could not verify host memory budget (no proxmox probe); ensure the host has headroom before growing masters",
			"grow_master_mb", opts.GrowMasterMemoryMB, "masters", numMasters)
		return nil
	}
	peak := projectCompactPeakMiB(
		opts.HostAllocatedMiB,
		r.Cfg.Topology.Workers.MemoryMB,
		r.Cfg.Topology.ControlPlane.MemoryMB,
		opts.GrowMasterMemoryMB,
		numWorkers, numMasters,
	)
	if err := validateMemoryBudget(opts.HostTotalMiB, opts.HostAllocatedMiB, peak-opts.HostAllocatedMiB); err != nil {
		return &errtypes.ConfigError{Msg: err.Error()}
	}
	return nil
}

// growMaster resizes one master to the compact grow target, passing the
// worker-discounted host allocation so the resize budget guard sees the memory
// freed by the workers removed so far rather than the pre-compaction total.
func (r *Runner) growMaster(ctx context.Context, master string, allocatedMiB int, opts CompactOptions) error {
	if err := r.Resize(ctx, ResizeScope{Node: master}, ResizeOptions{
		MemoryMB:         opts.GrowMasterMemoryMB,
		HostTotalMiB:     opts.HostTotalMiB,
		HostAllocatedMiB: allocatedMiB,
		Acknowledge:      opts.Acknowledge,
	}); err != nil {
		return fmt.Errorf("compact: grow master %s: %w", master, err)
	}
	return nil
}

// compactHybridError explains the mixed state left when a worker removal fails
// mid-compact: the control plane is already schedulable with compact ingress
// applied but only some workers are gone. Re-running compact is safe — already
// removed workers stay gone.
func (r *Runner) compactHybridError(removed, total int, failedNode string, cause error) error {
	return fmt.Errorf(
		"compact: remove worker %s (%d of %d workers already removed; the control plane is already schedulable with the compact IngressController applied — resolve the cause and re-run 'okdctl cluster compact' to remove the remaining %d worker(s), already-removed workers stay gone): %w",
		failedNode, removed, total, total-removed, cause,
	)
}

// compactPlan builds the informed-confirmation summary from the preflight
// verdicts: one worker delete per verdict (in removal order) carrying its OSD /
// ingress placement and any guard/plan refusal, plus the master-grow target.
func compactPlan(pf compactPreflight, clusterName string, opts CompactOptions) OpPlan {
	nodes := make([]PlanNode, 0, len(pf.verdicts))
	for i := range pf.verdicts {
		v := &pf.verdicts[i]
		nodes = append(nodes, PlanNode{
			Name:      v.node,
			Role:      nodetypes.RoleWorker,
			TFAddress: workerAddress(v.index),
			Action:    terraform.PlanActionDelete,
			OSDs:      v.osds,
			Ingress:   v.ingress,
			Blocked:   v.blocked,
		})
	}
	return OpPlan{
		Op:                 OpCompact,
		Cluster:            clusterName,
		Nodes:              nodes,
		DrainTimeout:       defaultDrainTimeout,
		GrowMasterMemoryMB: opts.GrowMasterMemoryMB,
		IngressReplicas:    opts.IngressReplicas,
	}
}

// reportCompactPlan prints the ordered dry-run action list with per-node
// verdicts: the control-plane step, each worker removal (with its OSD/ingress
// placement and gate verdict), and the interleaved master grows.
func (r *Runner) reportCompactPlan(ctx context.Context, workers, masters []string, pf compactPreflight, opts CompactOptions) {
	replicas := opts.IngressReplicas
	if replicas <= 0 {
		replicas = 2
	}
	r.Log.Info("node: dry-run — compact plan (no changes made)",
		"workers_to_remove", len(workers), "masters", len(masters), "grow_master_mb", opts.GrowMasterMemoryMB)
	r.reportEtcdDryRunVerdict(ctx)
	r.Log.Info("node: dry-run — step: make control plane schedulable and apply compact ingress",
		"ingress_replicas", replicas)

	masterGrows := 0
	for i, v := range pf.verdicts {
		if v.blocked != nil {
			r.Log.Warn("node: dry-run — remove worker WOULD BE REFUSED",
				"node", v.node, "tf_address", workerAddress(v.index),
				"osds", len(v.osds), "ingress_pods_here", len(v.ingress), "err", v.blocked)
		} else {
			r.Log.Info("node: dry-run — remove worker",
				"node", v.node, "tf_address", workerAddress(v.index), "plan", "delete",
				"verdict", "ok", "osds", len(v.osds), "ingress_pods_here", len(v.ingress))
		}
		if opts.GrowMasterMemoryMB > 0 && masterGrows < len(masters) && i < len(masters) {
			r.Log.Info("node: dry-run — grow master", "node", masters[masterGrows], "memory_mb", opts.GrowMasterMemoryMB)
			masterGrows++
		}
	}
	for ; opts.GrowMasterMemoryMB > 0 && masterGrows < len(masters); masterGrows++ {
		r.Log.Info("node: dry-run — grow master", "node", masters[masterGrows], "memory_mb", opts.GrowMasterMemoryMB)
	}
	if pf.memErr != nil {
		r.Log.Warn("node: dry-run — master grow WOULD EXCEED host memory budget", "err", pf.memErr)
	}
}

// reportEtcdDryRunVerdict runs a single non-blocking etcd probe and prints it
// as a dry-run verdict line. Unlike the real path's waitEtcdHealthy gate it
// never blocks and never fails the preview: a degraded quorum is surfaced so
// the operator knows the real compact would wait for it up to EtcdGateTimeout.
func (r *Runner) reportEtcdDryRunVerdict(ctx context.Context) {
	h, err := r.Cluster.EtcdHealthy(ctx)
	switch {
	case err != nil:
		r.Log.Warn("node: dry-run — etcd: UNHEALTHY — real compact will wait up to 10m", "reason", err.Error())
	case !h.Healthy:
		reason := h.Reason
		if reason == "" {
			reason = "not healthy"
		}
		r.Log.Warn("node: dry-run — etcd: UNHEALTHY — real compact will wait up to 10m", "reason", reason)
	default:
		r.Log.Info("node: dry-run — etcd: healthy")
	}
}

func (r *Runner) enableSchedulableAndIngress(ctx context.Context, replicas int) error {
	if err := r.Cluster.SetMastersSchedulable(ctx, true); err != nil {
		return err
	}
	if replicas <= 0 {
		replicas = 2
	}
	manifest, err := templates.RenderCompactIngress(templates.CompactIngressData{Replicas: replicas})
	if err != nil {
		return &errtypes.ClusterError{Msg: "render compact ingress", Err: err}
	}
	if err := r.Cluster.Apply(ctx, []byte(manifest)); err != nil {
		return &errtypes.ClusterError{Msg: "apply compact ingress controller", Err: err}
	}
	r.Log.Info("node: control plane schedulable and compact ingress applied", "ingress_replicas", replicas)
	return nil
}

func workersByIndexDesc(nodes []cluster.NodeDetail, log *slog.Logger) []string {
	return namesByIndex(nodes, nodetypes.RoleWorker, false, log)
}

func mastersByIndexAsc(nodes []cluster.NodeDetail, log *slog.Logger) []string {
	return namesByIndex(nodes, nodetypes.RoleMaster, true, log)
}

// namesByIndex silently drops any node whose name has no numeric suffix from
// the returned plan (cluster.NodeIndex can't place it); log warns per node so
// a hand-added or oddly named worker doesn't vanish from a compact plan
// without a trace.
func namesByIndex(nodes []cluster.NodeDetail, role nodetypes.NodeRole, ascending bool, log *slog.Logger) []string {
	type ni struct {
		name string
		idx  int
	}
	var items []ni
	for _, n := range nodes {
		if n.Role != role {
			continue
		}
		idx, ok := cluster.NodeIndex(n.Name)
		if !ok {
			log.Warn("node: skipping node with no numeric suffix from compact plan", "node", n.Name, "role", role)
			continue
		}
		items = append(items, ni{name: n.Name, idx: idx})
	}
	sort.Slice(items, func(i, j int) bool {
		if ascending {
			return items[i].idx < items[j].idx
		}
		return items[i].idx > items[j].idx
	})
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.name
	}
	return names
}
