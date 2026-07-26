package node

import (
	"context"
	"fmt"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// RemoveOptions tunes a worker removal.
type RemoveOptions struct {
	// ForceStorage permits removal even when the worker holds rook-ceph OSDs
	// (whose CEPH-DATA disk is destroyed with the VM).
	ForceStorage bool
	SkipDrain    bool
	DrainTimeout string
	// Acknowledge overrides a stranded marker left by a different op/target so
	// RemoveWorker proceeds fresh instead of refusing; see beginOp.
	Acknowledge bool
}

// RemoveWorker removes the named worker: guards, cordon, drain, targeted
// terraform delete (plan-gated to exactly worker[N-1]), then Kubernetes node
// delete. It does not touch HAProxy — the completion log points the operator
// at the manual backend-refresh steps instead. On any failure the node is
// left cordoned. Only the highest-numbered worker is removable (count-index
// rule). An interrupted run resumes at its recorded step (see beginOp),
// skipping the guards/confirm gate — they assume a clean baseline that no
// longer holds once a prior attempt has partially mutated the cluster.
func (r *Runner) RemoveWorker(ctx context.Context, target string, opts RemoveOptions) error {
	// A dry-run previews a fresh plan and mutates nothing, so resume is
	// irrelevant: skip beginOp entirely so a stranded foreign marker previews
	// rather than refusing.
	var marker *OpMarker
	if !r.DryRun {
		var err error
		marker, err = r.beginOp(OpRemove, func(m *OpMarker) bool { return m.Target == target }, opts.Acknowledge)
		if err != nil {
			return err
		}
	}
	resuming := marker != nil
	resumeStep := Step("")
	if resuming {
		resumeStep = marker.Step
	}

	workerCount := r.Cfg.Topology.Workers.Count
	// A digit-less target must fail here rather than silently parse to idx 0:
	// the resume path skips validateWorkerRemovable, and idx drives both the
	// terraform address and the worker_count persist below.
	idx, ok := cluster.NodeIndex(target)
	if !ok {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("cannot derive a terraform index from node name %q", target)}
	}

	var osdHere, ingressHere []string
	if !resuming {
		nodes, err := r.Cluster.ListNodes(ctx)
		if err != nil {
			return &errtypes.ClusterError{Msg: msgListNodes, Err: err}
		}
		if err := validateWorkerRemovable(nodes, target, workerCount); err != nil {
			return &errtypes.ConfigError{Msg: err.Error(), Err: err}
		}
		osdHere, ingressHere, err = r.removeGuards(ctx, nodes, target, opts.ForceStorage)
		if err != nil {
			return err
		}
	}

	plan := OpPlan{
		Op:           OpRemove,
		Cluster:      r.Cfg.Cluster.Name,
		DrainTimeout: opts.DrainTimeout,
		Nodes: []PlanNode{{
			Name:      target,
			Role:      nodetypes.RoleWorker,
			TFAddress: workerAddress(idx),
			Action:    terraform.PlanActionDelete,
			OSDs:      osdHere,
			Ingress:   ingressHere,
		}},
	}

	// worker[idx] leaves the config exactly when worker_count drops to idx (the
	// workers 0..idx-1 remain). Using idx directly is absolute, so a resumed
	// re-run over an already-decremented config computes the same target rather
	// than decrementing again. On a fresh run idx == workerCount-1 (the
	// removable-worker guard enforces it), so the delete preview is unchanged;
	// persisting is forbidden in dry-run, so this override also drives a
	// truthful preview without a config write.
	countVars := map[string]string{tfVarWorkerCount: strconv.Itoa(idx)}

	if r.DryRun {
		r.preview(&plan)
		return r.targetedApply(ctx, workerAddress(idx), terraform.PlanActionDelete, countVars, resuming)
	}

	if !resuming {
		proceed, err := r.confirm(ctx, &plan)
		if err != nil {
			return err
		}
		if !proceed {
			r.Log.Info("node: remove cancelled", "node", target)
			return ErrDeclined
		}
	}

	r.Log.Info("node: removing worker", "node", target)

	if !opts.SkipDrain {
		if err := r.cordonAndDrain(ctx, OpRemove, target, opts.DrainTimeout, false, resumeStep); err != nil {
			return err
		}
	}

	if shouldRunStep(StepTFApply, resumeStep) {
		if err := markStep(r.marker(), OpRemove, target, StepTFApply, r.RunID, r.Cfg.Cluster.Name); err != nil {
			return err
		}
		if err := r.targetedApply(ctx, workerAddress(idx), terraform.PlanActionDelete, countVars, resuming); err != nil {
			return err
		}

		// Persist only after the apply has verifiably landed (real apply or an
		// already-at-target resume). The assignment is absolute (Count = idx,
		// the removed worker's stable index), not a relative decrement, so a
		// crash after this persist but before the delete-node marker advances
		// re-runs it as a no-op on resume rather than decrementing a second time
		// and understating the topology (which would destroy a healthy worker on
		// the next deploy).
		r.Cfg.Topology.Workers.Count = idx
		if err := r.persistTopology(); err != nil {
			return &errtypes.ClusterError{Msg: msgPersistTopology, Err: err}
		}
	}

	if shouldRunStep(StepDeleteK8s, resumeStep) {
		if err := markStep(r.marker(), OpRemove, target, StepDeleteK8s, r.RunID, r.Cfg.Cluster.Name); err != nil {
			return err
		}
		if err := r.Cluster.DeleteNode(ctx, target); err != nil {
			return err
		}
	}

	// If the removed worker held OSDs (--force-storage / compaction), destroying
	// its CEPH-DATA disk triggers a rebalance. Wait for structural Ceph health
	// before returning so a compact loop never starts draining the next worker
	// while a replica is still missing. No-op on non-Ceph clusters.
	if err := r.waitCephHealthy(ctx, "post-remove-"+target); err != nil {
		return err
	}

	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}
	r.Log.Info("node: worker removed", "node", target)
	r.Log.Info("node: if haproxy fronts this cluster, its config still lists this worker; 1) edit /etc/haproxy/haproxy.cfg as root and drop its 'server' line from the http-backend and https-backend sections 2) validate with 'haproxy -c -f /etc/haproxy/haproxy.cfg' 3) apply with 'systemctl restart haproxy'",
		"node", target)
	return nil
}

// cordonAndDrain cordons then drains node, writing an op marker (under op — the
// caller's identity, not a hardcoded one, so a snapshot and a remove sharing
// this helper never leave a marker the other could be mistaken for) before each
// step, and gating each step via resumeStep so a run resuming exactly at
// StepDrain (cordon already landed before the crash) re-runs only the drain.
func (r *Runner) cordonAndDrain(ctx context.Context, op Op, node, timeout string, force bool, resumeStep Step) error {
	stop := r.startProgress(fmt.Sprintf("cordoning and draining %s", node))
	defer stop()

	if shouldRunStep(StepCordon, resumeStep) {
		if err := markStep(r.marker(), op, node, StepCordon, r.RunID, r.Cfg.Cluster.Name); err != nil {
			return err
		}
		if err := r.Cluster.Cordon(ctx, node); err != nil {
			return err
		}
	}
	if shouldRunStep(StepDrain, resumeStep) {
		if err := markStep(r.marker(), op, node, StepDrain, r.RunID, r.Cfg.Cluster.Name); err != nil {
			return err
		}
		if err := r.Cluster.Drain(ctx, node, cluster.DrainOptions{
			IgnoreDaemonsets: true,
			DeleteEmptyDir:   true,
			Force:            force,
			Timeout:          timeout,
		}); err != nil {
			// remove/resize resume their own marker on a plain re-run; a
			// snapshot op refuses its own marker as foreign, so its retry
			// advice must name the acknowledge flag or it sends the operator
			// into a refusal loop.
			retry := "re-run to retry"
			if op == OpSnapshot {
				retry = "re-run with --acknowledge-interrupted-op to retry"
			}
			return &errtypes.ClusterError{Msg: fmt.Sprintf("drain %s (node left cordoned; %s)", node, retry), Err: err}
		}
	}
	return nil
}

// removeGuards runs the read-only storage and ingress guards for a worker
// removal with a single pod fetch per selector, returning the OSD and router
// pods on target so the confirmation box can report them without re-querying.
// A blocked guard returns its refusal; the pod slices are then unused.
func (r *Runner) removeGuards(ctx context.Context, nodes []cluster.NodeDetail, target string, force bool) (osdHere, ingressHere []string, err error) {
	osdPods, err := r.Cluster.PodsForSelector(ctx, "", "app=rook-ceph-osd")
	if err != nil {
		return nil, nil, &errtypes.ClusterError{Msg: "storage guard: list rook-ceph osd pods", Err: err}
	}
	routerPods, err := r.Cluster.PodsForSelector(ctx, "openshift-ingress", "")
	if err != nil {
		return nil, nil, &errtypes.ClusterError{Msg: "ingress guard: list router pods", Err: err}
	}
	osdHere = podNamesOnNode(osdPods, target)
	ingressHere = podNamesOnNode(routerPods, target)

	if err := storageGuardVerdict(target, osdHere, force, r.Log); err != nil {
		return nil, nil, err
	}
	if err := r.checkIngressGuard(ctx, nodes, routerPods, target); err != nil {
		return nil, nil, err
	}
	return osdHere, ingressHere, nil
}

func (r *Runner) checkIngressGuard(ctx context.Context, nodes []cluster.NodeDetail, routerPods []cluster.PodPlacement, target string) error {
	workers := workerNameSet(nodes)
	onWorkers := ingressPodsOnWorkers(routerPods, workers)
	if len(onWorkers) == 0 {
		return nil
	}
	schedulable, err := r.Cluster.MastersSchedulable(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: "ingress guard: read mastersSchedulable", Err: err}
	}
	if schedulable {
		return nil
	}
	return &errtypes.ConfigError{Msg: fmt.Sprintf(
		"router pods run on worker nodes (%s) and the control plane is not schedulable; draining %s would leave ingress nowhere to reschedule. Set mastersSchedulable=true and apply the compact IngressController first (see 'okdctl cluster compact'), or move ingress off workers.",
		joinPodNames(onWorkers), target,
	)}
}
