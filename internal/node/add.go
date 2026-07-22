package node

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// AddOptions tunes a worker batch add.
type AddOptions struct {
	// Count is how many new workers to add. The batch occupies the next
	// Count terraform count indices after the persisted worker count.
	Count int
	// HostTotalMiB / HostAllocatedMiB feed the memory-budget guard when a
	// read-only Proxmox probe supplied them; both zero skips the numeric check.
	HostTotalMiB     int
	HostAllocatedMiB int
	// Acknowledge overrides a stranded marker left by a different op/target so
	// AddWorkers proceeds fresh instead of refusing; see beginOp.
	Acknowledge bool
}

// preflightIgnitionArtifacts verifies the setup phase's ignition assets —
// worker.ign and the ignition TLS cert — are still on disk. Node add reuses
// both as-is rather than regenerating them, so `okdctl cleanup --kind
// web-only` removing them must be caught here before any mutation runs; a
// `cleanup full` run is already caught by node ops' upstream missing-
// kubeconfig guard.
func (r *Runner) preflightIgnitionArtifacts() error {
	ignPath := filepath.Join(phase.ClusterConfigDir(r.WorkDir), "worker.ign")
	if !system.FileExists(ignPath) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf(
			"worker.ign not found at %s; re-run setup (e.g. 'okdctl deploy') to regenerate it before adding a node", ignPath)}
	}

	certPath, _ := setup.IgnitionCertPaths(r.ProjectRoot)
	if !system.FileExists(certPath) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf(
			"ignition tls cert not found at %s; re-run setup (e.g. 'okdctl deploy') to regenerate it before adding a node", certPath)}
	}
	return nil
}

// validateWorkerCountMatchesCluster guards the count-index scheme node add
// relies on: the new batch's terraform indices start at
// topology.workers.count, so that value must match how many worker nodes the
// cluster actually reports, or the new indices could collide with a node the
// persisted config doesn't know about.
func validateWorkerCountMatchesCluster(nodes []cluster.NodeDetail, want int) error {
	got := 0
	for _, n := range nodes {
		if n.Role == nodetypes.RoleWorker {
			got++
		}
	}
	if got != want {
		return fmt.Errorf("topology.workers.count is %d but the cluster reports %d worker node(s); reconcile config with 'okdctl node list' before adding", want, got)
	}
	return nil
}

// addPlan builds the informed-confirmation summary for a worker batch add:
// one create per new index, starting at startIdx.
func addPlan(clusterName string, startIdx, count int) OpPlan {
	nodes := make([]PlanNode, count)
	for i := range nodes {
		idx := startIdx + i
		nodes[i] = PlanNode{
			Name:      fmt.Sprintf("%s-worker%d", clusterName, idx),
			Role:      nodetypes.RoleWorker,
			TFAddress: workerAddress(idx),
			Action:    terraform.PlanActionCreate,
		}
	}
	return OpPlan{
		Op:      OpAdd,
		Cluster: clusterName,
		Nodes:   nodes,
	}
}

// AddWorkers guards, plans, and — outside dry-run — confirms a batch add of
// opts.Count new workers. This covers add's read-only sequence only: the
// mutating sequence (per-node ISO build/upload, ignition-server revive with
// a deferred teardown, targeted create, CSR approval, and wait-join) lands in
// a follow-up change on top of this skeleton; see the comment at the bottom
// of this function for the exact stopping point.
func (r *Runner) AddWorkers(ctx context.Context, opts AddOptions) error {
	if opts.Count < 1 {
		return &errtypes.ConfigError{Msg: "add: --count must be >= 1"}
	}
	if r.Cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: "add: proxmox provider configuration required"}
	}

	startIdx := r.Cfg.Topology.Workers.Count
	newNodeName := fmt.Sprintf("%s-worker%d", r.Cfg.Cluster.Name, startIdx)

	// A dry-run previews a fresh plan and mutates nothing, so resume is
	// irrelevant: skip beginOp entirely so a stranded foreign marker previews
	// rather than refusing (mirrors RemoveWorker/Resize).
	var marker *OpMarker
	if !r.DryRun {
		var err error
		marker, err = r.beginOp(OpAdd, func(m *OpMarker) bool { return m.Target == newNodeName }, opts.Acknowledge)
		if err != nil {
			return err
		}
	}
	resuming := marker != nil

	if err := r.preflightIgnitionArtifacts(); err != nil {
		return err
	}

	nodes, err := r.Cluster.ListNodes(ctx)
	if err != nil {
		return &errtypes.ClusterError{Msg: "list nodes", Err: err}
	}
	if err := validateWorkerCountMatchesCluster(nodes, startIdx); err != nil {
		return &errtypes.ConfigError{Msg: err.Error()}
	}

	delta := r.Cfg.Topology.Workers.MemoryMB * opts.Count
	if opts.HostTotalMiB > 0 {
		if err := validateMemoryBudget(opts.HostTotalMiB, opts.HostAllocatedMiB, delta); err != nil {
			return &errtypes.ConfigError{Msg: err.Error()}
		}
	} else if delta > 0 {
		r.Log.Warn("node: could not verify host memory budget (no proxmox probe); ensure the host has headroom before adding nodes",
			"delta_mib_total", delta, "nodes", opts.Count)
	}

	plan := addPlan(r.Cfg.Cluster.Name, startIdx, opts.Count)

	// A dry-run never persists, so every per-node plan gate previews against
	// the same plan-time overrides widened to the batch's final worker count —
	// the module validates length(worker_isos) >= worker_count, so widening
	// one without the other would fail the preview against the smaller lists
	// still on disk in terraform.tfvars.
	if r.DryRun {
		r.preview(&plan)
		total := startIdx + opts.Count
		planVars := map[string]string{
			"worker_count": strconv.Itoa(total),
			"worker_isos":  setup.WorkerISOsPlanVar(r.Cfg.Provider.Proxmox.ISOStorage, total),
		}
		for i := range plan.Nodes {
			if err := r.targetedApply(ctx, plan.Nodes[i].TFAddress, terraform.PlanActionCreate, planVars, resuming); err != nil {
				return err
			}
		}
		return nil
	}

	if !resuming {
		proceed, err := r.confirm(ctx, &plan)
		if err != nil {
			return err
		}
		if !proceed {
			r.Log.Info("node: add cancelled", "count", opts.Count)
			return ErrDeclined
		}
	}

	// Guards, plan, and confirm end here. The mutating sequence — per-node ISO
	// build/upload, ignition-server revive with a deferred teardown, targeted
	// create, CSR approval, and wait-join — is not implemented yet.
	return nil
}
