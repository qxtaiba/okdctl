package node

import (
	"context"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// StartOptions tunes a cluster start.
type StartOptions struct {
	// Acknowledge overrides a stranded marker left by any other in-flight
	// op — start is non-resumable and composes nothing, so unlike compact it
	// has no inner op to exempt; see Runner.refuseForeignMarker.
	Acknowledge bool
}

// Start powers the cluster back on: every master first (as one batch, no
// inter-master ready-wait), then every worker, then waits for all nodes to
// report Ready while approving kubelet CSRs each poll, and finally uncordons.
//
// Node enumeration is CONFIG-DRIVEN, not ListNodes: the Kubernetes API is
// hosted by the very control-plane VMs Start has not powered on yet, so the
// synthetic names master0..N-1 / worker0..N-1 come from cfg.Topology counts.
// Masters power on as one batch because etcd needs a quorum majority up before
// any single member is healthy — waiting on master0 alone would hang forever.
// Only once the API is reachable (after the readiness wait) does Start switch
// to real ListNodes names for the uncordon.
func (r *Runner) Start(ctx context.Context, opts StartOptions) error {
	if !r.DryRun {
		if r.Power == nil {
			return &errtypes.ClusterError{Msg: "cluster start needs Proxmox API access to power on VMs, but no Proxmox credentials are available"}
		}
		if err := r.refuseForeignMarker(opts.Acknowledge); err != nil {
			return err
		}
	}
	r.warnIfHAManaged("start")

	cpCount := r.Cfg.Topology.ControlPlane.Count
	workerCount := r.Cfg.Topology.Workers.Count
	masters := syntheticNodeNames(nodetypes.RoleMaster, cpCount)
	workers := syntheticNodeNames(nodetypes.RoleWorker, workerCount)

	plan := clusterPowerPlan(OpStart, r.Cfg.Cluster.Name, workers, masters)

	if r.DryRun {
		r.preview(&plan)
		return nil
	}

	if err := r.confirmOrDecline(ctx, &plan, "node: cluster start cancelled", "cluster", r.Cfg.Cluster.Name); err != nil {
		return err
	}

	r.Log.Info("node: starting cluster", "masters", cpCount, "workers", workerCount)

	if err := r.mark(OpStart, r.Cfg.Cluster.Name, StepPowerOn); err != nil {
		return err
	}

	if err := r.powerOnRole(ctx, nodetypes.RoleMaster, cpCount, "powering on control-plane nodes"); err != nil {
		return err
	}
	if err := r.powerOnRole(ctx, nodetypes.RoleWorker, workerCount, "powering on worker nodes"); err != nil {
		return err
	}

	if err := r.waitClusterReadyWithCSRApproval(ctx); err != nil {
		return err
	}

	if err := r.uncordonAll(ctx); err != nil {
		return err
	}

	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}
	r.Log.Info("node: cluster started", "masters", cpCount, "workers", workerCount)
	return nil
}

// syntheticNodeNames enumerates the role0..count-1 names owned by
// nodetypes.ClusterNode.Name so cluster start can enumerate nodes from
// config before the API that would list them is up.
func syntheticNodeNames(role nodetypes.NodeRole, count int) []string {
	names := make([]string, count)
	for i := range count {
		names[i] = nodetypes.ClusterNode{Role: role, Index: i}.Name()
	}
	return names
}

// powerOnRole starts every VM of a role as one batch — sequential StartVM calls
// with no ready-wait between them. A per-node wait here would deadlock the
// control plane: etcd only forms a quorum once a majority of masters are up, so
// blocking on the first master before starting the rest can never converge.
func (r *Runner) powerOnRole(ctx context.Context, role nodetypes.NodeRole, count int, desc string) error {
	if count == 0 {
		return nil
	}
	stop := r.startProgress(desc)
	defer stop()

	for i := range count {
		vmNode, vmid := r.vmTarget(role, i)
		if err := r.Power.StartVM(ctx, vmNode, vmid); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("start vm %d (%s%d)", vmid, role, i), Err: err}
		}
	}
	return nil
}

// waitClusterReadyWithCSRApproval blocks until every node reports Ready or the
// gate times out, approving pending kubelet CSRs on each poll so a cluster
// restarted after its kubelet client certs rotated can rejoin unattended. The
// two failure modes have independent log-once gates (ListNodes when the API is
// still coming up, ApprovePendingCSRs when the API is up but CSR listing hiccups)
// so neither floods the log across the full timeout window.
func (r *Runner) waitClusterReadyWithCSRApproval(ctx context.Context) error {
	stop := r.startProgress("waiting for cluster to become ready")
	defer stop()

	listWarn := logutil.NewDedupWarner(r.Log)
	approveWarn := logutil.NewDedupWarner(r.Log)
	var lastReason string
	ok := func(ctx context.Context) bool {
		nodes, err := r.Cluster.ListNodes(ctx)
		if err != nil {
			listWarn.Warn(err.Error(), "node: cluster api not reachable yet", "err", err)
			lastReason = "cluster api not reachable"
			return false
		}
		listWarn.Reset()

		if approved, aerr := r.Cluster.ApprovePendingCSRs(ctx); aerr != nil {
			approveWarn.Warn(aerr.Error(), "node: csr approval check failed", "err", aerr)
		} else {
			approveWarn.Reset()
			if approved > 0 {
				r.Log.Info("node: approved pending csrs", "approved", approved)
			}
		}

		if len(nodes) == 0 {
			lastReason = "no nodes registered yet"
			return false
		}
		for _, n := range nodes {
			if !n.Ready {
				lastReason = fmt.Sprintf("node %s not ready", n.Name)
				return false
			}
		}
		return true
	}
	if err := system.WaitForWithTimeout(ctx, "cluster", "ready", ok, r.ClusterReadyTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("cluster did not become ready: %s", lastReason), Err: err}
	}
	return nil
}

// uncordonAll returns every node to service after a start, addressing them by
// their real ListNodes names — safe here because the readiness wait has already
// confirmed the API is up.
func (r *Runner) uncordonAll(ctx context.Context) error {
	stop := r.startProgress("uncordoning all nodes")
	defer stop()

	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: msgListNodes, Err: err}
	}
	for _, n := range nodes {
		if err := r.Cluster.Uncordon(ctx, n.Name); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("uncordon %s", n.Name), Err: err}
		}
	}
	return nil
}
