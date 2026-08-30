package node

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// ignitionTeardownTimeout bounds AddWorkers' detached teardown so a wedged
// systemctl call can't hang process exit.
const ignitionTeardownTimeout = 30 * time.Second

// AddOptions tunes a worker batch add.
type AddOptions struct {
	// Count is how many new workers to add, occupying the next Count terraform indices.
	Count int
	// HostTotalMiB / HostAllocatedMiB feed the memory-budget guard; both zero skips it.
	HostTotalMiB     int
	HostAllocatedMiB int
	// Acknowledge overrides a stranded marker from a different op/target; see beginOp.
	Acknowledge bool
}

// preflightIgnitionArtifacts verifies worker.ign and the ignition TLS cert —
// the SOURCE copies ReviveIgnitionServer re-deploys — are still on disk.
func (r *Runner) preflightIgnitionArtifacts() error {
	ignPath := filepath.Join(workspace.ClusterConfigDir(r.workDir), "worker.ign")
	if !system.FileExists(ignPath) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf(
			"worker.ign not found at %s; re-run setup (e.g. 'okdctl deploy') to regenerate it before adding a node", ignPath,
		)}
	}

	certPath, _ := provision.IgnitionCertPaths(r.projectRoot)
	if !system.FileExists(certPath) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf(
			"ignition tls cert not found at %s; re-run setup (e.g. 'okdctl deploy') to regenerate it before adding a node", certPath,
		)}
	}
	return nil
}

// validateWorkerCountMatchesCluster guards the count-index scheme: the new
// batch starts at topology.workers.count, which must match the cluster's
// actual worker count or new indices could collide with an unknown node.
func validateWorkerCountMatchesCluster(nodes []cluster.NodeDetail, want int) error {
	got := 0
	for _, n := range nodes {
		if n.Role == nodetypes.RoleWorker {
			got++
		}
	}
	if got != want {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("topology.workers.count is %d but the cluster reports %d worker node(s); reconcile config with 'okdctl node list' before adding", want, got)}
	}
	return nil
}

// addPlan builds the confirmation summary: one create per new index from startIdx.
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

// AddWorkers guards, plans, confirms, and executes a batch add of opts.Count
// new workers, per node building/uploading its ISO, applying a plan-gated
// create, and waiting for it to join and report Ready. An interrupted batch
// resumes at its recorded node/step (see beginOp), skipping guards/confirm
// since a prior partial run breaks their clean-baseline assumption.
func (r *Runner) AddWorkers(ctx context.Context, opts AddOptions) error {
	if opts.Count < 1 {
		return &errtypes.ConfigError{Msg: "add: --count must be >= 1"}
	}
	if r.Cfg.Provider.Proxmox == nil {
		return &errtypes.ConfigError{Msg: "add: proxmox provider configuration required"}
	}

	startIdx := r.Cfg.Topology.Workers.Count
	endIdx := startIdx + opts.Count - 1
	batchLabel := r.workerName(startIdx)

	// Dry-run mutates nothing, so skip beginOp: a stranded marker previews rather than refuses.
	var marker *OpMarker
	if !r.DryRun {
		var err error
		marker, err = r.beginOp(OpAdd, addBatchMatch(startIdx, endIdx), opts.Acknowledge)
		if err != nil {
			return err
		}
	}
	resuming := marker != nil

	plan := addPlan(r.Cfg.Cluster.Name, startIdx, opts.Count)

	// Guards assume a clean baseline, so run them only on a fresh op.
	if !resuming {
		if err := r.preflightIgnitionArtifacts(); err != nil {
			return err
		}
		nodes, err := r.Cluster.ListNodes(ctx)
		if err != nil {
			return &errtypes.ClusterError{Msg: msgListNodes, Err: err}
		}
		if err := validateWorkerCountMatchesCluster(nodes, startIdx); err != nil {
			return err
		}

		delta := r.Cfg.Topology.Workers.MemoryMB * opts.Count
		if opts.HostTotalMiB > 0 {
			if err := validateMemoryBudget(opts.HostTotalMiB, opts.HostAllocatedMiB, delta); err != nil {
				return err
			}
		} else if delta > 0 {
			r.Log.Warn("node: could not verify host memory budget (no proxmox probe); ensure the host has headroom before adding nodes",
				"delta_mib_total", delta, "nodes", opts.Count)
		}
	}

	// Every per-node preview widens worker_count/worker_isos together to the
	// batch's final total — the module asserts length(worker_isos) >= worker_count.
	if r.DryRun {
		r.preview(&plan)
		total := startIdx + opts.Count
		planVars := map[string]string{
			tfVarWorkerCount: strconv.Itoa(total),
			"worker_isos":    provision.WorkerISOsPlanVar(r.Cfg.Provider.Proxmox.ISOStorage, total),
		}
		for i := range plan.Nodes {
			if err := r.targetedApply(ctx, plan.Nodes[i].TFAddress, terraform.PlanActionCreate, planVars, resuming); err != nil {
				return err
			}
		}
		return nil
	}

	if !resuming {
		if err := r.confirmOrDecline(ctx, &plan, "node: add cancelled", "count", opts.Count); err != nil {
			return err
		}
	}

	r.Log.Info("node: adding workers", "count", opts.Count)

	// One batch-scoped join window: revive now, defer teardown before any VM
	// exists so it fires on every exit path. Teardown doesn't rewrite the op
	// marker, so a failed batch keeps its per-node resume position.
	if err := r.mark(OpAdd, batchLabel, StepIgnitionUp); err != nil {
		return err
	}
	// Detached from ctx: a cancelled ctx would fail every systemctl call
	// before it starts, leaving httpd serving the pull-secret-bearing
	// worker.ign indefinitely. WithTimeout is the only bound left — keep it.
	defer func() {
		tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ignitionTeardownTimeout)
		defer cancel()
		if terr := r.Ignition.TeardownIgnitionServer(tctx); terr != nil {
			r.Log.Warn("node: ignition server teardown failed — the cluster pull secret may still be served on port 443; stop it manually ('systemctl stop httpd') and verify with 'systemctl status httpd'",
				"err", terr)
		}
	}()
	if err := r.Ignition.ReviveIgnitionServer(ctx, r.Cfg, r.projectRoot, workspace.ClusterConfigDir(r.workDir)); err != nil {
		return &errtypes.ClusterError{Msg: "revive ignition server", Err: err}
	}

	for idx := startIdx; idx <= endIdx; idx++ {
		if err := r.addOneWorker(ctx, idx, marker); err != nil {
			return err
		}
	}

	// Persist only once the whole batch has joined, so a crash mid-batch
	// leaves config/tfvars at the pre-add count and a resume recomputes the
	// same [startIdx, endIdx] range instead of shifting it forward.
	r.Cfg.Topology.Workers.Count = endIdx + 1
	if err := r.persistTopology(); err != nil {
		return &errtypes.ClusterError{Msg: msgPersistTopology, Err: err}
	}

	if err := clearOpMarker(r.marker()); err != nil {
		r.Log.Warn("node: op marker cleanup failed", "err", err)
	}
	r.Log.Info("node: workers added", "count", opts.Count, "from_index", startIdx)
	r.Log.Info("node: if haproxy fronts this cluster, add a 'server' line for each new worker to the http-backend and https-backend sections of /etc/haproxy/haproxy.cfg, validate with 'haproxy -c -f /etc/haproxy/haproxy.cfg', then apply with 'systemctl restart haproxy'",
		"count", opts.Count)
	return nil
}

// addBatchMatch matches an add marker whose worker index falls in [startIdx,
// endIdx] — the marker roams across the batch as each node is worked.
func addBatchMatch(startIdx, endIdx int) opMatch {
	return func(m *OpMarker) bool {
		i, ok := cluster.NodeIndex(m.Target)
		return ok && i >= startIdx && i <= endIdx
	}
}

func (r *Runner) workerName(idx int) string {
	return fmt.Sprintf("%s-worker%d", r.Cfg.Cluster.Name, idx)
}

// addOneWorker builds/uploads the new worker's ISO, applies the plan-gated
// create, and waits for it to join. marker (nil on a fresh run) makes a node
// preceding it skip whole, the marked node resume at its step, and later nodes run fresh.
func (r *Runner) addOneWorker(ctx context.Context, idx int, marker *OpMarker) error {
	name := r.workerName(idx)

	resumeStep := Step("")
	if marker != nil {
		if marker.Target == name {
			resumeStep = marker.Step
		} else if mi, ok := cluster.NodeIndex(marker.Target); ok && mi > idx {
			r.Log.Info("node: worker already joined before interruption — skipping", "node", name)
			return nil
		}
		// mi < idx (or unparseable): not yet reached before the crash — run fresh.
	}

	// In-memory only bump so BuildCustomISOs renders this node's ISO; the
	// single disk write is the after-loop persistTopology, so a crash here strands nothing.
	total := idx + 1
	r.Cfg.Topology.Workers.Count = total
	planVars := map[string]string{
		tfVarWorkerCount: strconv.Itoa(total),
		"worker_isos":    provision.WorkerISOsPlanVar(r.Cfg.Provider.Proxmox.ISOStorage, total),
	}
	resuming := marker != nil

	if err := r.runStep(OpAdd, name, StepBuildISO, resumeStep, func() error {
		if err := r.ISO.BuildCustomISOs(ctx, r.Cfg, r.Provision); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("build iso for %s", name), Err: err}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := r.runStep(OpAdd, name, StepUploadISO, resumeStep, func() error {
		if err := r.ISO.UploadCustomISOsToProxmox(ctx, r.Cfg, r.Provision); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("upload iso for %s", name), Err: err}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := r.runStep(OpAdd, name, StepTFApply, resumeStep, func() error {
		return r.targetedApply(ctx, workerAddress(idx), terraform.PlanActionCreate, planVars, resuming)
	}); err != nil {
		return err
	}
	if err := r.runStep(OpAdd, name, StepWaitJoin, resumeStep, func() error {
		return r.waitWorkerJoined(ctx, name)
	}); err != nil {
		return err
	}
	r.Log.Info("node: worker added", "node", name)
	return nil
}

// waitWorkerJoined blocks until node registers and reports Ready, approving
// pending kubelet CSRs each poll since a joining node needs its bootstrap CSR approved first.
func (r *Runner) waitWorkerJoined(ctx context.Context, node string) error {
	stop := r.startProgress(fmt.Sprintf("waiting for %s to join and become ready", node))
	defer stop()

	approveWarn := logutil.NewDedupWarner(r.Log)
	var lastReason string
	ok := func(ctx context.Context) bool {
		if approved, aerr := r.Cluster.ApprovePendingCSRs(ctx); aerr != nil {
			approveWarn.Warn(aerr.Error(), "node: csr approval check failed", "err", aerr)
		} else {
			approveWarn.Reset()
			if approved > 0 {
				r.Log.Info("node: approved pending csrs", "approved", approved)
			}
		}

		nodes, err := r.Cluster.ListNodes(ctx)
		if err != nil {
			lastReason = "cluster api not reachable"
			return false
		}
		for _, n := range nodes {
			if n.Name == node {
				if n.Ready {
					return true
				}
				lastReason = "node registered but not yet ready"
				return false
			}
		}
		lastReason = "node not yet registered"
		return false
	}
	if err := system.WaitForWithTimeout(ctx, "node", node+"-join", ok, r.NodeReadyTimeout, r.Log); err != nil {
		return &errtypes.ClusterError{Msg: fmt.Sprintf("worker %s did not join and become Ready: %s", node, lastReason), Err: err}
	}
	return nil
}
