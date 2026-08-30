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
	// (destroying their CEPH-DATA disk).
	ForceStorage bool
	SkipDrain    bool
	DrainTimeout string
	// Acknowledge overrides a stranded marker from a different op/target so
	// RemoveWorker proceeds fresh; see beginOp.
	Acknowledge bool
}

// RemoveWorker removes the named worker: guards, cordon, drain, a targeted
// terraform delete (gated to worker[N-1]), then Kubernetes node delete; HAProxy
// is untouched. Failure leaves the node cordoned; a resumed run re-enters at
// its recorded step (see beginOp), skipping guards/confirm since a prior
// partial run breaks their clean-baseline assumption.
func (r *Runner) RemoveWorker(ctx context.Context, target string, opts RemoveOptions) error {
	// Dry-run mutates nothing, so resume is irrelevant: skip beginOp so a
	// stranded marker previews rather than refuses.
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
	// A digit-less target must fail here (not silently parse to idx 0): resume
	// skips validateWorkerRemovable, and idx drives the tf address and
	// worker_count persist below.
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
			return err
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

	// countVars uses idx directly (absolute, not a decrement) so a resumed
	// re-run over an already-decremented config computes the same target.
	countVars := map[string]string{tfVarWorkerCount: strconv.Itoa(idx)}

	if r.DryRun {
		r.preview(&plan)
		return r.targetedApply(ctx, workerAddress(idx), terraform.PlanActionDelete, countVars, resuming)
	}

	if !resuming {
		if err := r.confirmOrDecline(ctx, &plan, "node: remove cancelled", "node", target); err != nil {
			return err
		}
	}

	r.Log.Info("node: removing worker", "node", target)

	if !opts.SkipDrain {
		if err := r.cordonAndDrain(ctx, OpRemove, target, opts.DrainTimeout, false, resumeStep); err != nil {
			return err
		}
	}

	if err := r.runStep(OpRemove, target, StepTFApply, resumeStep, func() error {
		if err := r.targetedApply(ctx, workerAddress(idx), terraform.PlanActionDelete, countVars, resuming); err != nil {
			return err
		}

		// Count = idx is absolute, so a crash between this persist and the
		// delete-node marker re-runs as a no-op instead of double-decrementing.
		r.Cfg.Topology.Workers.Count = idx
		if err := r.persistTopology(); err != nil {
			return &errtypes.ClusterError{Msg: msgPersistTopology, Err: err}
		}
		return nil
	}); err != nil {
		return err
	}

	if err := r.runStep(OpRemove, target, StepDeleteK8s, resumeStep, func() error {
		return r.Cluster.DeleteNode(ctx, target)
	}); err != nil {
		return err
	}

	// Destroying an OSD's disk triggers a rebalance; wait for Ceph health so a
	// compact loop never drains mid-recovery.
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

// cordonAndDrain cordons then drains node, marking each step under the caller's own op.
func (r *Runner) cordonAndDrain(ctx context.Context, op Op, node, timeout string, force bool, resumeStep Step) error {
	stop := r.startProgress(fmt.Sprintf("cordoning and draining %s", node))
	defer stop()

	if err := r.runStep(op, node, StepCordon, resumeStep, func() error {
		return r.Cluster.Cordon(ctx, node)
	}); err != nil {
		return err
	}
	return r.runStep(op, node, StepDrain, resumeStep, func() error {
		if err := r.Cluster.Drain(ctx, node, cluster.DrainOptions{
			IgnoreDaemonsets: true,
			DeleteEmptyDir:   true,
			Force:            force,
			Timeout:          timeout,
		}); err != nil {
			// snapshot refuses its own marker as foreign, so its retry advice
			// must name --acknowledge-interrupted-op.
			retry := "re-run to retry"
			if op == OpSnapshot {
				retry = "re-run with --acknowledge-interrupted-op to retry"
			}
			return &errtypes.ClusterError{Msg: fmt.Sprintf("drain %s (node left cordoned; %s)", node, retry), Err: err}
		}
		return nil
	})
}

// removeGuards runs the read-only storage/ingress guards, returning target's
// OSD/router pods so confirm can reuse them.
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
