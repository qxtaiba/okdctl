package node

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// CompactOptions tunes cluster compaction.
type CompactOptions struct {
	// IngressReplicas: compact IngressController replica count (masters serve
	// ingress once workers are gone)
	IngressReplicas int
	// GrowMasterMemoryMB > 0 resizes masters after removal, interleaved so
	// growth never precedes a freed worker
	GrowMasterMemoryMB int
	ForceStorage       bool
	// Host memory budget (read-only Proxmox probe); zero skips the check
	HostTotalMiB     int
	HostAllocatedMiB int
	// Acknowledge overrides a stranded marker from a different op/target,
	// threaded into every inner RemoveWorker/Resize call. Only OpRemove/OpResize
	// markers pass through unrefused (may be compact's own in-flight inner op);
	// any other op family is refused up front.
	Acknowledge bool
}

// compactVerdict is one worker's read-only preflight result (blocked nil = clear).
type compactVerdict struct {
	node    string
	index   int
	osds    []string
	ingress []string
	blocked error
}

// compactPreflight aggregates preflight results across workers plus the memory
// projection; blockErr nil means clear.
type compactPreflight struct {
	verdicts []compactVerdict
	memErr   error
	blockErr error
}

// Compact consolidates the cluster onto its control plane: make masters
// schedulable, apply the compact IngressController, then remove workers
// top-down, interleaving an optional master grow after each removal.
// It records no op marker of its own — RemoveWorker/Resize record
// OpRemove/OpResize against the node they mutate, so a crash resumes via the
// inner op's own marker.
func (r *Runner) Compact(ctx context.Context, opts CompactOptions) error {
	if !r.DryRun {
		// OpRemove/OpResize markers are left to the inner op's own resume; any
		// other op family is refused here before mutation.
		if err := r.refuseForeignMarker(opts.Acknowledge, OpRemove, OpResize); err != nil {
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
		r.reportCompactPlan(ctx, masters, pf, opts)
		return pf.blockErr
	}
	if pf.blockErr != nil {
		return pf.blockErr
	}

	if err := r.confirmOrDecline(ctx, &plan, "node: compact cancelled", "cluster", r.Cfg.Cluster.Name); err != nil {
		return err
	}

	// Suppress inner gates: consent was already granted above, so no
	// mid-teardown prompt can abort a half-executed sequence.
	r.preConsented = true
	defer func() { r.preConsented = false }()

	r.Log.Info("node: compacting cluster", "workers", len(workers), "masters", len(masters))

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
		// Interleave: grow the next master after freeing a worker so allocation
		// never peaks above pre-compaction levels.
		if opts.GrowMasterMemoryMB > 0 && masterGrows < len(masters) && i < len(masters) {
			if err := r.growMaster(ctx, masters[masterGrows], allocated, opts); err != nil {
				return err
			}
			masterGrows++
		}
	}

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

// runPreflightCompact wraps preflightCompact in one reporter span rather than a
// spinner flickering per worker.
func (r *Runner) runPreflightCompact(ctx context.Context, workers, masters []string, opts CompactOptions) (compactPreflight, error) {
	stop := r.startProgress(fmt.Sprintf("gating removal plan for %d worker node(s)", len(workers)))
	defer stop()
	return r.preflightCompact(ctx, workers, masters, opts)
}

// preflightCompact runs read-only guards per worker plus the memory projection;
// storage blocks, ingress is remediated by compact itself.
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

// assertWorkerDeletable plan-gates deleting worker[idx] via a plan-time -var
// override, without applying; the saved plan is dropped immediately.
func (r *Runner) assertWorkerDeletable(ctx context.Context, idx int) error {
	countVars := map[string]string{tfVarWorkerCount: strconv.Itoa(idx)}
	_, _, cleanup, err := r.planTargeted(ctx, workerAddress(idx), terraform.PlanActionDelete, countVars, false)
	cleanup()
	return err
}

// projectCompactMemory refuses the plan if interleaved master grows would
// exceed the host memory budget; a missing probe only warns.
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
	return validateMemoryBudget(opts.HostTotalMiB, opts.HostAllocatedMiB, peak-opts.HostAllocatedMiB)
}

// growMaster resizes a master using the worker-discounted host allocation so
// the budget guard sees memory already freed.
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

// compactHybridError explains a mid-compact failure: the control plane is
// already schedulable with only some workers removed; re-running is safe.
func (r *Runner) compactHybridError(removed, total int, failedNode string, cause error) error {
	return fmt.Errorf(
		"compact: remove worker %s (%d of %d workers already removed; the control plane is already schedulable with the compact IngressController applied — resolve the cause and re-run 'okdctl cluster compact' to remove the remaining %d worker(s), already-removed workers stay gone): %w",
		failedNode, removed, total, total-removed, cause,
	)
}

// compactPlan builds the confirmation summary from preflight verdicts: one
// worker delete per verdict plus the master-grow target.
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

// reportCompactPlan prints the ordered dry-run action list with per-node verdicts.
func (r *Runner) reportCompactPlan(ctx context.Context, masters []string, pf compactPreflight, opts CompactOptions) {
	r.Log.Info("node: dry-run — compact plan (no changes made)",
		"workers_to_remove", len(pf.verdicts), "masters", len(masters), "grow_master_mb", opts.GrowMasterMemoryMB)
	r.reportEtcdDryRunVerdict(ctx)
	r.Log.Info("node: dry-run — step: make control plane schedulable and apply compact ingress",
		"ingress_replicas", compactIngressReplicas(opts.IngressReplicas))

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

// reportEtcdDryRunVerdict runs one non-blocking etcd probe for the dry-run
// preview, unlike the real path's blocking waitEtcdHealthy gate.
func (r *Runner) reportEtcdDryRunVerdict(ctx context.Context) {
	h, err := r.Cluster.EtcdHealthy(ctx)
	switch {
	case err != nil:
		r.Log.Warn("node: dry-run — etcd: UNHEALTHY — real compact will wait for quorum health",
			"wait_up_to", r.EtcdGateTimeout, "err", err)
	case !h.Healthy:
		reason := h.Reason
		if reason == "" {
			reason = "not healthy"
		}
		r.Log.Warn("node: dry-run — etcd: UNHEALTHY — real compact will wait for quorum health",
			"wait_up_to", r.EtcdGateTimeout, "reason", reason)
	default:
		r.Log.Info("node: dry-run — etcd: healthy")
	}
}

// compactIngressReplicas defaults non-positive replicas to 2.
func compactIngressReplicas(replicas int) int {
	if replicas <= 0 {
		return 2
	}
	return replicas
}

func (r *Runner) enableSchedulableAndIngress(ctx context.Context, replicas int) error {
	if err := r.Cluster.SetMastersSchedulable(ctx, true); err != nil {
		return err
	}
	replicas = compactIngressReplicas(replicas)
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

// namesByIndex drops nodes without a numeric-suffix name (can't be placed),
// warning per node so none vanish silently.
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
			log.Warn("node: skipping node with no numeric suffix from compact plan", "node", n.Name, "role", string(role))
			continue
		}
		items = append(items, ni{name: n.Name, idx: idx})
	}
	slices.SortFunc(items, func(a, b ni) int {
		if ascending {
			return cmp.Compare(a.idx, b.idx)
		}
		return cmp.Compare(b.idx, a.idx)
	})
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.name
	}
	return names
}
