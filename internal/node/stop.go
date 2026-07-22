package node

import (
	"context"
	"fmt"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// signerWarnWindow mirrors doctor's day-2 kubelet-signer expiry threshold
// (internal/doctor/cluster.go). Kept local rather than shared: node and
// doctor read the same cluster.Client fact independently and a shared
// constant would couple two packages that have no other dependency on
// each other.
const signerWarnWindow = 30 * 24 * time.Hour

// StopOptions tunes a cluster stop.
type StopOptions struct {
	// Acknowledge overrides a stranded marker left by any other in-flight
	// op — stop is non-resumable and composes nothing, so unlike compact it
	// has no inner op to exempt; see Runner.refuseForeignMarker.
	Acknowledge bool
}

// Stop shuts the cluster down: cordon every node, then gracefully power off
// every worker (ascending) followed by every master (ascending) through the
// Proxmox API. It never drains — with the whole cluster stopping there is
// nowhere left to reschedule a pod, so a drain would only spin until its
// own timeout. Restart with 'okdctl cluster start'.
func (r *Runner) Stop(ctx context.Context, opts StopOptions) error {
	if !r.DryRun {
		if r.Power == nil {
			return &errtypes.ClusterError{Msg: "cluster stop needs Proxmox API access to power off VMs, but no Proxmox credentials are available"}
		}
		if err := r.refuseForeignMarker(opts.Acknowledge); err != nil {
			return err
		}
	}

	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: "list nodes", Err: err}
	}
	workers := namesByIndex(nodes, nodetypes.RoleWorker, true, r.Log)
	masters := mastersByIndexAsc(nodes, r.Log)

	plan := clusterPowerPlan(OpStop, r.Cfg.Cluster.Name, workers, masters)

	r.reportSignerExpiry(ctx)

	if r.DryRun {
		r.preview(&plan)
		return nil
	}

	proceed, err := r.confirm(ctx, &plan)
	if err != nil {
		return err
	}
	if !proceed {
		r.Log.Info("node: cluster stop cancelled", "cluster", r.Cfg.Cluster.Name)
		return ErrDeclined
	}

	if err := r.cordonAll(ctx, workers, masters); err != nil {
		return err
	}

	for _, w := range workers {
		if err := r.stopOneNode(ctx, w, nodetypes.RoleWorker); err != nil {
			return err
		}
	}
	for _, m := range masters {
		if err := r.stopOneNode(ctx, m, nodetypes.RoleMaster); err != nil {
			return err
		}
	}

	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}
	r.Log.Info("node: cluster stopped", "workers", len(workers), "masters", len(masters))
	return nil
}

// clusterPowerPlan builds the read-only plan summary shared by stop and start,
// each node carrying PlanActionNoop since these ops power a VM rather than
// change its terraform resource. Ordering follows the op's power sequence so
// the preview reads in execution order: stop takes workers before masters,
// start takes masters before workers (etcd quorum first).
func clusterPowerPlan(op Op, clusterName string, workers, masters []string) OpPlan {
	nodes := make([]PlanNode, 0, len(workers)+len(masters))
	appendRole := func(names []string, role nodetypes.NodeRole) {
		for _, n := range names {
			nodes = append(nodes, PlanNode{Name: n, Role: role, Action: terraform.PlanActionNoop})
		}
	}
	if op == OpStart {
		appendRole(masters, nodetypes.RoleMaster)
		appendRole(workers, nodetypes.RoleWorker)
	} else {
		appendRole(workers, nodetypes.RoleWorker)
		appendRole(masters, nodetypes.RoleMaster)
	}
	return OpPlan{Op: op, Cluster: clusterName, Nodes: nodes}
}

// reportSignerExpiry logs the kube-apiserver-to-kubelet-signer's remaining
// validity before the confirm gate, mirroring doctor's severity ladder
// (internal/doctor/cluster.go): kubelet certificate rotation needs the signer
// valid, and its expiry does not pause while the cluster is stopped, so an
// operator stopping the cluster should see the same runway doctor would show.
func (r *Runner) reportSignerExpiry(ctx context.Context) {
	notAfter, err := r.Cluster.SignerNotAfter(ctx)
	if err != nil {
		r.Log.Warn("node: could not read kubelet signer expiry", "err", err)
		return
	}
	remaining := time.Until(notAfter)
	days := int(remaining.Hours() / 24)
	date := notAfter.Format("2006-01-02")
	switch {
	case remaining <= 0:
		r.Log.Warn("node: kube-apiserver-to-kubelet-signer already expired", "expired", date)
	case remaining < signerWarnWindow:
		r.Log.Warn("node: kube-apiserver-to-kubelet-signer expires soon", "days_remaining", days, "expires", date)
	default:
		r.Log.Info("node: kube-apiserver-to-kubelet-signer expiry checked", "days_remaining", days, "expires", date)
	}
}

// cordonAll cordons every node before any shutdown begins, so a mid-stop
// failure leaves the whole cluster unschedulable rather than a partially
// cordoned mix that could still take a workload. Cordon is idempotent, so a
// resumed stop simply re-cordons already-cordoned nodes.
func (r *Runner) cordonAll(ctx context.Context, workers, masters []string) error {
	stop := r.startProgress("cordoning all nodes")
	defer stop()

	for _, n := range workers {
		if err := r.Cluster.Cordon(ctx, n); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("cordon %s", n), Err: err}
		}
	}
	for _, n := range masters {
		if err := r.Cluster.Cordon(ctx, n); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("cordon %s", n), Err: err}
		}
	}
	return nil
}

// stopOneNode gracefully powers off one already-cordoned node. A failure
// leaves the node cordoned — Stop runs no drain, so the cordon is the only
// signal an operator has that this node did not power off cleanly.
func (r *Runner) stopOneNode(ctx context.Context, node string, role nodetypes.NodeRole) error {
	stop := r.startProgress(fmt.Sprintf("shutting down %s", node))
	defer stop()

	idx, _ := cluster.NodeIndex(node)
	vmNode, vmid := r.vmTarget(role, idx)

	if err := markStep(r.marker(), OpStop, node, StepShutdown, r.RunID, r.Cfg.Cluster.Name); err != nil {
		return err
	}
	if err := r.Power.ShutdownVM(ctx, vmNode, vmid); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("shut down vm %d for node %s (node left cordoned; re-run 'okdctl cluster stop' to retry)", vmid, node), Err: err}
	}
	return nil
}
