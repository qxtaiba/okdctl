package node

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// ignitionTeardownTimeout bounds the detached teardown context (see AddWorkers)
// so a wedged systemctl call cannot hang process exit indefinitely.
const ignitionTeardownTimeout = 30 * time.Second

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
// worker.ign and the ignition TLS cert — are still on disk. These are the
// SOURCE copies the revive re-deploys into the web root, so this guards
// against their loss (e.g. a manual wipe of cluster-config); the served
// web-root copies that `okdctl cleanup --kind web-only` removes are healed
// by ReviveIgnitionServer's re-deploy, not checked here. A `cleanup full`
// run is already caught by node ops' upstream missing-kubeconfig guard.
func (r *Runner) preflightIgnitionArtifacts() error {
	ignPath := filepath.Join(phase.ClusterConfigDir(r.WorkDir), "worker.ign")
	if !system.FileExists(ignPath) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf(
			"worker.ign not found at %s; re-run setup (e.g. 'okdctl deploy') to regenerate it before adding a node", ignPath,
		)}
	}

	certPath, _ := setup.IgnitionCertPaths(r.ProjectRoot)
	if !system.FileExists(certPath) {
		return &errtypes.ConfigError{Msg: fmt.Sprintf(
			"ignition tls cert not found at %s; re-run setup (e.g. 'okdctl deploy') to regenerate it before adding a node", certPath,
		)}
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

// AddWorkers guards, plans, confirms, and executes a batch add of opts.Count
// new workers. Outside dry-run it reopens the ignition server for one
// batch-scoped join window (revive once, deferred teardown once on any exit),
// then per new index builds/uploads the node's ISO, applies a plan-gated
// exactly-one-create, and waits for the node to join and report Ready while
// approving its kubelet CSRs. It does not touch HAProxy — the completion log
// points the operator at the manual backend-add steps. An interrupted batch
// resumes at its recorded node/step (see beginOp): already-joined nodes are
// skipped, the marked node re-enters at its step, and not-yet-reached nodes
// run fresh. Resume skips the guards/confirm gate — they assume a clean
// baseline that no longer holds once a prior attempt partially mutated things.
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

	// A dry-run previews a fresh plan and mutates nothing, so resume is
	// irrelevant: skip beginOp entirely so a stranded foreign marker previews
	// rather than refusing (mirrors RemoveWorker/Resize).
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

	// Guards assume a clean baseline; a resume has already moved past it, so
	// run them only on a fresh op (mirrors RemoveWorker/Resize).
	if !resuming {
		if err := r.preflightIgnitionArtifacts(); err != nil {
			return err
		}
		nodes, err := r.Cluster.ListNodes(ctx)
		if err != nil {
			return &errtypes.ClusterError{Msg: msgListNodes, Err: err}
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
	}

	// A dry-run never persists, so every per-node plan gate previews against
	// the same plan-time overrides widened to the batch's final worker count —
	// the module validates length(worker_isos) >= worker_count, so widening
	// one without the other would fail the preview against the smaller lists
	// still on disk in terraform.tfvars.
	if r.DryRun {
		r.preview(&plan)
		total := startIdx + opts.Count
		planVars := map[string]string{
			tfVarWorkerCount: strconv.Itoa(total),
			"worker_isos":    setup.WorkerISOsPlanVar(r.Cfg.Provider.Proxmox.ISOStorage, total),
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

	// One batch-scoped join window: revive the ignition server once, and defer
	// its teardown NOW — before any VM exists — so it fires on success,
	// failure, timeout, or panic and the window is never left open. Teardown is
	// idempotent (stop+disable httpd, not uninstall) and does NOT rewrite the
	// op marker, so a failed batch keeps its per-node resume position. The
	// window spans the whole --count N batch: Apache stays up across every
	// node's build/apply, which for N>1 is a deliberately widened window.
	if err := markStep(r.marker(), OpAdd, batchLabel, StepIgnitionUp, r.RunID, r.Cfg.Cluster.Name); err != nil {
		return err
	}
	// Detached from ctx's cancellation: on SIGINT during the join wait, ctx is
	// already cancelled by the time this runs, and every systemctl call under
	// a cancelled ctx fails before it even starts — leaving httpd serving
	// worker.ign (which embeds the pull secret) indefinitely.
	// context.WithoutCancel drops BOTH the cancellation signal and any
	// deadline; the explicit WithTimeout is the only bound on a wedged
	// systemctl — do not remove it as redundant.
	defer func() {
		tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ignitionTeardownTimeout)
		defer cancel()
		if terr := r.Ignition.TeardownIgnitionServer(tctx); terr != nil {
			r.Log.Warn("node: ignition server teardown failed — the cluster pull secret may still be served on port 443; stop it manually ('systemctl stop httpd') and verify with 'systemctl status httpd'",
				"err", terr)
		}
	}()
	if err := r.Ignition.ReviveIgnitionServer(ctx, r.Cfg, r.ProjectRoot, phase.ClusterConfigDir(r.WorkDir)); err != nil {
		return &errtypes.ClusterError{Msg: "revive ignition server", Err: err}
	}

	for idx := startIdx; idx <= endIdx; idx++ {
		if err := r.addOneWorker(ctx, idx, marker); err != nil {
			return err
		}
	}

	// Persist the widened topology only once the whole batch has joined — a
	// crash mid-batch must leave config/tfvars at the pre-add count so a
	// resumed run recomputes the same batch range (startIdx, endIdx) rather
	// than shifting it forward. Mirrors Resize's after-the-loop persist. A
	// re-created VM on resume is covered by targetedApply's alreadyAtTarget
	// classification, so re-entering the apply after this persist is a no-op.
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

// addBatchMatch matches an add marker whose recorded worker index falls in the
// batch's [startIdx, endIdx] range, so a resumed --count N batch reattaches to
// a marker recorded against ANY node in the batch, not only its first — the
// marker roams across the batch as each node is worked.
func addBatchMatch(startIdx, endIdx int) OpMatch {
	return func(m *OpMarker) bool {
		i, ok := cluster.NodeIndex(m.Target)
		return ok && i >= startIdx && i <= endIdx
	}
}

func (r *Runner) workerName(idx int) string {
	return fmt.Sprintf("%s-worker%d", r.Cfg.Cluster.Name, idx)
}

// addOneWorker builds and uploads the new worker's ISO, applies the plan-gated
// exactly-one-create, and waits for the node to join. marker is the batch's
// resume marker (nil on a fresh run): a node whose index precedes the marked
// node already joined before the interruption and is skipped whole; the marked
// node re-enters at its recorded step; a node past the marked one runs fresh.
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

	// Absolute in-memory bump so BuildCustomISOs renders this node's ISO and
	// the after-loop persist writes the batch's final count regardless of the
	// resume point. In-memory only; the single disk write is the after-loop
	// persistTopology, so a crash here strands nothing.
	total := idx + 1
	r.Cfg.Topology.Workers.Count = total
	planVars := map[string]string{
		tfVarWorkerCount: strconv.Itoa(total),
		"worker_isos":    setup.WorkerISOsPlanVar(r.Cfg.Provider.Proxmox.ISOStorage, total),
	}
	resuming := marker != nil

	if shouldRunStep(StepBuildISO, resumeStep) {
		if err := markStep(r.marker(), OpAdd, name, StepBuildISO, r.RunID, r.Cfg.Cluster.Name); err != nil {
			return err
		}
		if err := r.ISO.BuildCustomISOs(ctx, r.Cfg, r.SetupOpts); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("build iso for %s", name), Err: err}
		}
	}
	if shouldRunStep(StepUploadISO, resumeStep) {
		if err := markStep(r.marker(), OpAdd, name, StepUploadISO, r.RunID, r.Cfg.Cluster.Name); err != nil {
			return err
		}
		if err := r.ISO.UploadCustomISOsToProxmox(ctx, r.Cfg, r.SetupOpts); err != nil {
			return &errtypes.ClusterError{Msg: fmt.Sprintf("upload iso for %s", name), Err: err}
		}
	}
	if shouldRunStep(StepTFApply, resumeStep) {
		if err := markStep(r.marker(), OpAdd, name, StepTFApply, r.RunID, r.Cfg.Cluster.Name); err != nil {
			return err
		}
		if err := r.targetedApply(ctx, workerAddress(idx), terraform.PlanActionCreate, planVars, resuming); err != nil {
			return err
		}
	}
	if shouldRunStep(StepWaitJoin, resumeStep) {
		if err := markStep(r.marker(), OpAdd, name, StepWaitJoin, r.RunID, r.Cfg.Cluster.Name); err != nil {
			return err
		}
		if err := r.waitWorkerJoined(ctx, name); err != nil {
			return err
		}
	}
	r.Log.Info("node: worker added", "node", name)
	return nil
}

// waitWorkerJoined blocks until node registers and reports Ready or the gate
// times out (NodeReadyTimeout). Each poll first approves any pending kubelet
// CSRs — a joining node cannot register until its bootstrap CSR is approved —
// then checks the node's readiness. The CSR-approval failure log is gated
// log-once-then-debug so a transient hiccup does not flood the wait window
// (mirrors install/monitor.go's poll loop).
func (r *Runner) waitWorkerJoined(ctx context.Context, node string) error {
	stop := r.startProgress(fmt.Sprintf("waiting for %s to join and become ready", node))
	defer stop()

	var lastApproveWarn, lastReason string
	ok := func(ctx context.Context) bool {
		if approved, aerr := r.Cluster.ApprovePendingCSRs(ctx); aerr != nil {
			if msg := aerr.Error(); msg != lastApproveWarn {
				r.Log.Warn("node: csr approval check failed", "err", aerr)
				lastApproveWarn = msg
			} else {
				r.Log.Debug("node: csr approval check failed (repeated)", "err", aerr)
			}
		} else {
			lastApproveWarn = ""
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
