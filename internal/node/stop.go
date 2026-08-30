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

// signerWarnWindow mirrors doctor's kubelet-signer expiry threshold (internal/doctor/cluster.go);
// kept local rather than shared to avoid coupling two otherwise-independent packages.
const signerWarnWindow = 30 * 24 * time.Hour

// StopOptions tunes a cluster stop.
type StopOptions struct {
	// Acknowledge overrides a stranded marker from any other in-flight op; see
	// Runner.refuseForeignMarker.
	Acknowledge bool
}

// Stop shuts the cluster down: cordon every node, then gracefully power off
// every worker (ascending) followed by every master (ascending). It never
// drains — with the whole cluster stopping there is nowhere to reschedule a
// pod, so a drain would only spin until its own timeout.
func (r *Runner) Stop(ctx context.Context, opts StopOptions) error {
	if !r.DryRun {
		if r.Power == nil {
			return &errtypes.ClusterError{Msg: "cluster stop needs Proxmox API access to power off VMs, but no Proxmox credentials are available"}
		}
		if err := r.refuseForeignMarker(opts.Acknowledge); err != nil {
			return err
		}
	}
	r.warnIfHAManaged("stop")

	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: msgListNodes, Err: err}
	}
	workers := namesByIndex(nodes, nodetypes.RoleWorker, true, r.Log)
	masters := mastersByIndexAsc(nodes, r.Log)

	plan := clusterPowerPlan(OpStop, r.Cfg.Cluster.Name, workers, masters)

	r.reportSignerExpiry(ctx)

	if r.DryRun {
		r.preview(&plan)
		return nil
	}

	if err := r.confirmOrDecline(ctx, &plan, "node: cluster stop cancelled", "cluster", r.Cfg.Cluster.Name); err != nil {
		return err
	}

	r.Log.Info("node: stopping cluster", "workers", len(workers), "masters", len(masters))

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

// clusterPowerPlan builds the read-only plan shared by stop/start (PlanActionNoop
// since these power a VM, not a terraform resource), ordered
// stop:workers-first, start:masters-first for quorum.
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

// reportSignerExpiry logs the kubelet signer's remaining validity before the
// confirm gate — its expiry does not pause while the cluster is stopped.
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
		r.Log.Warn("node: kube-apiserver-to-kubelet-signer already expired", "days_remaining", days, "expires", date)
	case remaining < signerWarnWindow:
		r.Log.Warn("node: kube-apiserver-to-kubelet-signer expires soon", "days_remaining", days, "expires", date)
	default:
		r.Log.Info("node: kube-apiserver-to-kubelet-signer expiry checked", "days_remaining", days, "expires", date)
	}
}

// warnIfHAManaged flags an okdctl power op against HA-managed masters — the
// CRM may restart what stop just shut down. Warn rather than refuse since
// the interaction is unverified and refusing would strand valid setups.
func (r *Runner) warnIfHAManaged(verb string) {
	if r.Cfg.Provider.Proxmox == nil || !r.Cfg.Provider.Proxmox.HAEnabled {
		return
	}
	r.Log.Warn("node: masters are proxmox-ha managed (ha_enabled); the ha manager may counteract this power operation — verify the cluster's power state afterwards, or set the ha request-state via pvesh first",
		"op", verb)
}

// cordonAll cordons every node before any shutdown begins, so a mid-stop
// failure never leaves a schedulable mix.
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

// stopOneNode gracefully powers off one already-cordoned node; a failure leaves
// it cordoned as the only failure signal.
func (r *Runner) stopOneNode(ctx context.Context, node string, role nodetypes.NodeRole) error {
	stop := r.startProgress(fmt.Sprintf("shutting down %s", node))
	defer stop()

	idx, _ := cluster.NodeIndex(node)
	vmNode, vmid := r.vmTarget(role, idx)

	if err := r.mark(OpStop, node, StepShutdown); err != nil {
		return err
	}
	if err := r.Power.ShutdownVM(ctx, vmNode, vmid); err != nil {
		// stop refuses its own stranded marker on a plain re-run, so retry
		// advice must name the acknowledge flag.
		return &errtypes.ClusterError{Msg: fmt.Sprintf("shut down vm %d for node %s (node left cordoned; re-run 'okdctl cluster stop' with --acknowledge-interrupted-op to retry)", vmid, node), Err: err}
	}
	return nil
}
