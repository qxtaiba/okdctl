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
	// Acknowledge overrides a stranded marker from any in-flight op; start is
	// non-resumable and composes nothing, unlike compact (see
	// refuseForeignMarker).
	Acknowledge bool
}

// Start powers the cluster back on: masters first as one batch, then workers,
// then waits for Ready while approving kubelet CSRs, then uncordons. Node
// enumeration is config-driven (not ListNodes) until the readiness wait
// confirms the API — hosted by the very control-plane VMs — is up.
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

// syntheticNodeNames enumerates role0..count-1 names from config, since the API
// that would list them isn't up yet.
func syntheticNodeNames(role nodetypes.NodeRole, count int) []string {
	names := make([]string, count)
	for i := range count {
		names[i] = nodetypes.ClusterNode{Role: role, Index: i}.Name()
	}
	return names
}

// powerOnRole starts every VM of a role as one batch; waiting on the first
// master would deadlock, since etcd needs quorum majority up first.
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

// waitClusterReadyWithCSRApproval blocks until every node is Ready, approving
// pending kubelet CSRs each poll so rotated certs can rejoin unattended;
// List/Approve failures use independent log-once gates.
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

// uncordonAll returns every node to service via real ListNodes names, safe once
// the readiness wait confirms the API is up.
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
