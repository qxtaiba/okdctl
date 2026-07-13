package node

import (
	"context"
	"fmt"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
)

// RemoveOptions tunes a worker removal.
type RemoveOptions struct {
	// ForceStorage permits removal even when the worker holds rook-ceph OSDs
	// (whose CEPH-DATA disk is destroyed with the VM).
	ForceStorage bool
	SkipDrain    bool
	DrainTimeout string
}

// RemoveWorker removes the named worker: guards, cordon, drain, targeted
// terraform delete (plan-gated to exactly worker[N-1]), then Kubernetes node
// delete. It does not touch HAProxy — the completion log points the operator
// at the manual backend-refresh steps instead. On any failure the node is
// left cordoned. Only the highest-numbered worker is removable (count-index
// rule).
func (r *Runner) RemoveWorker(ctx context.Context, target string, opts RemoveOptions) error {
	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: "list nodes", Err: err}
	}
	workerCount := r.Cfg.Topology.Workers.Count
	if err := validateWorkerRemovable(nodes, target, workerCount); err != nil {
		return &errtypes.ConfigError{Msg: err.Error()}
	}
	idx, _ := cluster.NodeIndex(target)

	if err := r.checkStorageGuard(ctx, target, opts.ForceStorage); err != nil {
		return err
	}
	if err := r.checkIngressGuard(ctx, nodes, target); err != nil {
		return err
	}

	// The dry-run delete preview must show the resource going away, but the
	// resource only leaves the config once worker_count drops. Persisting is
	// forbidden in dry-run, so feed the reduced count as a plan-time -var
	// override; without it the plan is a no-op and the gate reports an empty
	// plan (the fidelity bug this override fixes).
	countVars := map[string]string{"worker_count": strconv.Itoa(workerCount - 1)}

	if r.DryRun {
		r.Log.Info("node: dry-run — guards passed", "node", target, "tf_address", workerAddress(idx))
		return r.targetedApply(ctx, workerAddress(idx), terraform.PlanActionDelete, countVars)
	}

	if !opts.SkipDrain {
		if err := r.cordonAndDrain(ctx, target, opts.DrainTimeout, false); err != nil {
			return err
		}
	}

	// Persist the reduced topology before the apply so a crash between here and
	// apply leaves tfvars/config consistent with the intended delete; the apply
	// is idempotent on re-run (an already-deleted instance re-plans to no-op,
	// which the gate then reports as empty — see resume note in RemoveWorker docs).
	r.Cfg.Topology.Workers.Count = workerCount - 1
	if err := r.persistTopology(); err != nil {
		return &errtypes.ClusterError{Msg: "persist topology", Err: err}
	}

	if err := markStep(r.marker(), OpRemove, target, StepTFApply, r.RunID, r.Cfg.Cluster.Name); err != nil {
		return err
	}
	if err := r.targetedApply(ctx, workerAddress(idx), terraform.PlanActionDelete, countVars); err != nil {
		return err
	}

	if err := markStep(r.marker(), OpRemove, target, StepDeleteK8s, r.RunID, r.Cfg.Cluster.Name); err != nil {
		return err
	}
	if err := r.Cluster.DeleteNode(ctx, target); err != nil {
		return err
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

func (r *Runner) cordonAndDrain(ctx context.Context, node, timeout string, force bool) error {
	if err := markStep(r.marker(), OpRemove, node, StepCordon, r.RunID, r.Cfg.Cluster.Name); err != nil {
		return err
	}
	if err := r.Cluster.Cordon(ctx, node); err != nil {
		return err
	}
	if err := markStep(r.marker(), OpRemove, node, StepDrain, r.RunID, r.Cfg.Cluster.Name); err != nil {
		return err
	}
	if err := r.Cluster.Drain(ctx, node, cluster.DrainOptions{
		IgnoreDaemonsets: true,
		DeleteEmptyDir:   true,
		Force:            force,
		Timeout:          timeout,
	}); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("drain %s (node left cordoned; re-run to retry)", node), Err: err}
	}
	return nil
}

func (r *Runner) checkStorageGuard(ctx context.Context, target string, force bool) error {
	pods, err := r.Cluster.PodsForSelector(ctx, "", "app=rook-ceph-osd")
	if err != nil {
		return &errtypes.ClusterError{Msg: "storage guard: list rook-ceph osd pods", Err: err}
	}
	osds := osdPodsOnNode(pods, target)
	if len(osds) == 0 {
		return nil
	}
	if force {
		r.Log.Warn("node: --force-storage set; removing a node with live OSDs destroys their CEPH-DATA disk", "node", target, "osds", len(osds))
		return nil
	}
	return &errtypes.ConfigError{Msg: fmt.Sprintf(
		"%s holds %d rook-ceph OSD(s) (%v); removing it destroys its CEPH-DATA disk and loses that data. Migrate OSDs off it first, or re-run with --force-storage.",
		target, len(osds), osds)}
}

func (r *Runner) checkIngressGuard(ctx context.Context, nodes []cluster.NodeDetail, target string) error {
	routerPods, err := r.Cluster.PodsForSelector(ctx, "openshift-ingress", "")
	if err != nil {
		return &errtypes.ClusterError{Msg: "ingress guard: list router pods", Err: err}
	}
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
		joinPodNames(onWorkers), target)}
}
