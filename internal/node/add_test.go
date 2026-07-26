package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/setup"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// fakeISO records BuildCustomISOs/UploadCustomISOsToProxmox calls so a
// dry-run can be proven to trigger neither. events, when set, records the
// per-node "build"/"upload" ordering shared with the other fakes.
type fakeISO struct {
	buildCalls  int
	uploadCalls int
	buildErr    error
	events      *[]string
}

func (f *fakeISO) BuildCustomISOs(context.Context, *config.Config, *setup.Options) error {
	f.buildCalls++
	if f.events != nil {
		*f.events = append(*f.events, "build")
	}
	return f.buildErr
}

func (f *fakeISO) UploadCustomISOsToProxmox(context.Context, *config.Config, *setup.Options) error {
	f.uploadCalls++
	if f.events != nil {
		*f.events = append(*f.events, "upload")
	}
	return nil
}

// fakeIgnition records ReviveIgnitionServer/TeardownIgnitionServer calls so a
// dry-run can be proven to revive/teardown neither, and a real batch can be
// proven to revive/teardown exactly once. events records "revive"/"teardown".
// teardownErrAtCall/teardownHadDeadline snapshot the passed context's state
// synchronously inside the call (not after), since the caller's own deferred
// cancel() may fire immediately after TeardownIgnitionServer returns — reading
// ctx.Err() later would observe that cancel(), not the state teardown actually
// ran under.
type fakeIgnition struct {
	configureCalls      int
	teardownCalls       int
	teardownErrAtCall   error
	teardownHadDeadline bool
	events              *[]string
}

func (f *fakeIgnition) ReviveIgnitionServer(context.Context, *config.Config, string, string) error {
	f.configureCalls++
	if f.events != nil {
		*f.events = append(*f.events, "revive")
	}
	return nil
}

func (f *fakeIgnition) TeardownIgnitionServer(ctx context.Context) error {
	f.teardownCalls++
	f.teardownErrAtCall = ctx.Err()
	_, f.teardownHadDeadline = ctx.Deadline()
	if f.events != nil {
		*f.events = append(*f.events, "teardown")
	}
	return nil
}

const addTestClusterName = "mycluster"

func addTestConfig(workerCount, workerMemMB int) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = addTestClusterName
	cfg.Topology.Workers.Count = workerCount
	cfg.Topology.Workers.MemoryMB = workerMemMB
	// Keep DefaultConfig's full Proxmox block (Node, etc.) so a non-dry-run
	// batch's after-loop persistTopology can render terraform.tfvars; only the
	// ISO storage is overridden to the value the worker_isos assertions expect.
	cfg.Provider.Proxmox.ISOStorage = "iso-store"
	return cfg
}

// seedAddRunner builds a Runner rooted at a fresh temp dir, with sentinel
// tfvars/config files so a dry-run's zero-mutation contract can be checked,
// plus the fake ISO/ignition collaborators wired so their call counts prove
// (or disprove) mutation.
func seedAddRunner(t *testing.T, fc *fakeCluster, ftf *fakeTF, fiso *fakeISO, fign *fakeIgnition, cfg *config.Config) (r *Runner, tfvarsPath, cfgPath string) {
	t.Helper()
	dir := t.TempDir()
	tfvarsPath = filepath.Join(dir, "terraform.tfvars")
	cfgPath = filepath.Join(dir, "okdctl.yaml")
	if err := os.WriteFile(tfvarsPath, []byte("SENTINEL_TFVARS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("SENTINEL_CONFIG\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = &Runner{
		Cluster:     fc,
		TF:          ftf,
		ISO:         fiso,
		Ignition:    fign,
		Cfg:         cfg,
		ConfigPath:  cfgPath,
		ProjectRoot: dir,
		WorkDir:     filepath.Join(dir, "okd-install"),
		EnvDir:      dir,
		RunID:       "test-run",
		DryRun:      true,
		Log:         logutil.NopLogger,
	}
	return r, tfvarsPath, cfgPath
}

// writeIgnitionArtifacts creates worker.ign and the ignition TLS cert under
// r's WorkDir/ProjectRoot so preflightIgnitionArtifacts passes.
func writeIgnitionArtifacts(t *testing.T, r *Runner) {
	t.Helper()
	clusterDir := workspace.ClusterConfigDir(r.WorkDir)
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clusterDir, "worker.ign"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	certPath, _ := setup.IgnitionCertPaths(r.ProjectRoot)
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func addWorkerNodes(n int) []cluster.NodeDetail {
	nodes := make([]cluster.NodeDetail, n)
	for i := range nodes {
		nodes[i] = cluster.NodeDetail{Name: "worker-placeholder", Role: nodetypes.RoleWorker, Ready: true}
	}
	return nodes
}

func TestAddWorkersDryRunMakesNoMutation(t *testing.T) {
	fc := &fakeCluster{nodes: addWorkerNodes(2)}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(2, 16384)

	r, tfvars, cfgPath := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	writeIgnitionArtifacts(t, r)

	if err := r.AddWorkers(context.Background(), AddOptions{Count: 2}); err != nil {
		t.Fatalf("dry-run add: %v", err)
	}

	if fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 {
		t.Errorf("dry-run add mutated the cluster: cordon=%d drain=%d deleteNode=%d", fc.cordon, fc.drain, fc.deleteNode)
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("dry-run add applied terraform: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
	if fiso.buildCalls != 0 || fiso.uploadCalls != 0 {
		t.Errorf("dry-run add built/uploaded an iso: build=%d upload=%d", fiso.buildCalls, fiso.uploadCalls)
	}
	if fign.configureCalls != 0 || fign.teardownCalls != 0 {
		t.Errorf("dry-run add touched the ignition server: configure=%d teardown=%d", fign.configureCalls, fign.teardownCalls)
	}
	if ftf.planCalls != 2 {
		t.Errorf("dry-run add must plan-gate every new node: planCalls=%d want 2", ftf.planCalls)
	}
	// #C3 fidelity: the preview must widen worker_count AND worker_isos
	// together to the batch's final total (startIdx 2 + count 2 = 4), or the
	// module's length(worker_isos) >= worker_count assertion trips the plan.
	if ftf.lastVars["worker_count"] != "4" {
		t.Errorf("dry-run add plan did not widen worker_count: vars=%v", ftf.lastVars)
	}
	wantISOs := `["iso-store:iso/worker0.iso", "iso-store:iso/worker1.iso", "iso-store:iso/worker2.iso", "iso-store:iso/worker3.iso"]`
	if ftf.lastVars["worker_isos"] != wantISOs {
		t.Errorf("dry-run add plan did not widen worker_isos in lockstep: got %q want %q", ftf.lastVars["worker_isos"], wantISOs)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
	if _, err := os.Stat(filepath.Join(r.WorkDir, OpMarkerFileName)); !os.IsNotExist(err) {
		t.Error("dry-run add wrote an op marker")
	}
}

func TestAddWorkersMemoryBudgetRejected(t *testing.T) {
	fc := &fakeCluster{nodes: addWorkerNodes(2)}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(2, 40960)

	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	writeIgnitionArtifacts(t, r)

	// Host nearly full: adding 2 more 40 GiB workers must be refused.
	err := r.AddWorkers(context.Background(), AddOptions{
		Count:            2,
		HostTotalMiB:     96000,
		HostAllocatedMiB: 93000,
	})
	if err == nil {
		t.Fatal("want memory-budget refusal when the probe reports the host is full")
	}
	if ftf.planCalls != 0 {
		t.Errorf("budget refusal must precede any plan; planCalls=%d", ftf.planCalls)
	}
	if fiso.buildCalls != 0 || fign.configureCalls != 0 {
		t.Errorf("budget refusal must precede any mutation: build=%d configure=%d", fiso.buildCalls, fign.configureCalls)
	}
}

func TestAddWorkersPreflightArtifactMissingRejected(t *testing.T) {
	fc := &fakeCluster{nodes: addWorkerNodes(1)}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(1, 16384)

	// No writeIgnitionArtifacts call: worker.ign and the TLS cert are absent,
	// modelling `cleanup --kind web-only` having removed them.
	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)

	err := r.AddWorkers(context.Background(), AddOptions{Count: 1})
	if err == nil {
		t.Fatal("want preflight refusal when worker.ign/tls cert are missing")
	}
	if fc.listNodesCalls != 0 {
		t.Errorf("preflight must precede ListNodes; listNodesCalls=%d", fc.listNodesCalls)
	}
	if ftf.planCalls != 0 || fiso.buildCalls != 0 {
		t.Errorf("preflight refusal must precede any plan or mutation: planCalls=%d build=%d", ftf.planCalls, fiso.buildCalls)
	}
}

func TestAddWorkersPlanShape(t *testing.T) {
	cases := []struct {
		name        string
		startCount  int
		addCount    int
		wantIndices []int
	}{
		{name: "count 1", startCount: 2, addCount: 1, wantIndices: []int{2}},
		{name: "count 3", startCount: 2, addCount: 3, wantIndices: []int{2, 3, 4}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fc := &fakeCluster{nodes: addWorkerNodes(tt.startCount)}
			ftf := &fakeTF{action: terraform.PlanActionCreate}
			fiso := &fakeISO{}
			fign := &fakeIgnition{}
			cfg := addTestConfig(tt.startCount, 16384)

			r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
			writeIgnitionArtifacts(t, r)

			var captured *OpPlan
			r.Preview = func(p *OpPlan) { captured = p }

			if err := r.AddWorkers(context.Background(), AddOptions{Count: tt.addCount}); err != nil {
				t.Fatalf("dry-run add: %v", err)
			}
			if captured == nil {
				t.Fatal("preview was never called")
			}
			if len(captured.Nodes) != len(tt.wantIndices) {
				t.Fatalf("got %d plan nodes, want %d", len(captured.Nodes), len(tt.wantIndices))
			}
			for i, idx := range tt.wantIndices {
				n := captured.Nodes[i]
				if n.Name != fmt.Sprintf("mycluster-worker%d", idx) {
					t.Errorf("node %d name = %q, want %q", i, n.Name, fmt.Sprintf("mycluster-worker%d", idx))
				}
				if n.TFAddress != workerAddress(idx) {
					t.Errorf("node %d TFAddress = %q, want %q", i, n.TFAddress, workerAddress(idx))
				}
				if n.Action != terraform.PlanActionCreate {
					t.Errorf("node %d Action = %q, want %q", i, n.Action, terraform.PlanActionCreate)
				}
				if n.Role != nodetypes.RoleWorker {
					t.Errorf("node %d Role = %q, want %q", i, n.Role, nodetypes.RoleWorker)
				}
			}
			if captured.DestroysData() {
				t.Error("an add plan must never report DestroysData")
			}
		})
	}
}

// addExistingWorkers builds the pre-add worker roster the count-match guard
// checks against (names match r.workerName so the join wait can find them).
func addExistingWorkers() []cluster.NodeDetail {
	const n = 2
	nodes := make([]cluster.NodeDetail, n)
	for i := range nodes {
		nodes[i] = cluster.NodeDetail{
			Name:  fmt.Sprintf("%s-worker%d", addTestClusterName, i),
			Role:  nodetypes.RoleWorker,
			Ready: true,
		}
	}
	return nodes
}

func addAppearingWorkers(from, count int) []cluster.NodeDetail {
	nodes := make([]cluster.NodeDetail, count)
	for i := range nodes {
		nodes[i] = cluster.NodeDetail{
			Name:  fmt.Sprintf("%s-worker%d", addTestClusterName, from+i),
			Role:  nodetypes.RoleWorker,
			Ready: true,
		}
	}
	return nodes
}

// TestAddWorkersMutatingSequenceOrder locks the per-node ordering
// (build→upload→apply→join) and the batch-scoped ignition window (revive
// first, teardown last, each exactly once) for a fresh --count 2 batch.
func TestAddWorkersMutatingSequenceOrder(t *testing.T) {
	var events []string
	fc := &fakeCluster{
		nodes:               addExistingWorkers(),
		workersAppearAtCall: 2, // call 1 is the pre-add count-match guard
		appearingWorkers:    addAppearingWorkers(2, 2),
		events:              &events,
	}
	ftf := &fakeTF{action: terraform.PlanActionCreate, events: &events}
	fiso := &fakeISO{events: &events}
	fign := &fakeIgnition{events: &events}
	cfg := addTestConfig(2, 16384)

	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	writeIgnitionArtifacts(t, r)
	r.DryRun = false

	if err := r.AddWorkers(context.Background(), AddOptions{Count: 2}); err != nil {
		t.Fatalf("add: %v", err)
	}

	want := []string{"revive", "build", "upload", "apply", "join", "build", "upload", "apply", "join", "teardown"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Errorf("mutating sequence out of order:\n got %v\nwant %v", events, want)
	}
	if fign.configureCalls != 1 || fign.teardownCalls != 1 {
		t.Errorf("ignition window must revive/teardown exactly once: configure=%d teardown=%d", fign.configureCalls, fign.teardownCalls)
	}
	if cfg.Topology.Workers.Count != 4 {
		t.Errorf("batch add must persist the widened worker count: got %d want 4", cfg.Topology.Workers.Count)
	}
}

// TestAddWorkersReviveTeardownOncePerBatch proves the join window is opened and
// closed exactly once across a multi-node --count 3 batch (not once per node).
func TestAddWorkersReviveTeardownOncePerBatch(t *testing.T) {
	fc := &fakeCluster{
		nodes:               addExistingWorkers(),
		workersAppearAtCall: 2,
		appearingWorkers:    addAppearingWorkers(2, 3),
	}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(2, 16384)

	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	writeIgnitionArtifacts(t, r)
	r.DryRun = false

	if err := r.AddWorkers(context.Background(), AddOptions{Count: 3}); err != nil {
		t.Fatalf("add count 3: %v", err)
	}
	if fign.configureCalls != 1 {
		t.Errorf("revive must run exactly once for the whole batch: configure=%d", fign.configureCalls)
	}
	if fign.teardownCalls != 1 {
		t.Errorf("teardown must run exactly once for the whole batch: teardown=%d", fign.teardownCalls)
	}
	if fiso.buildCalls != 3 || fiso.uploadCalls != 3 || ftf.applyCalls != 3 {
		t.Errorf("each new node must build/upload/apply once: build=%d upload=%d apply=%d",
			fiso.buildCalls, fiso.uploadCalls, ftf.applyCalls)
	}
}

// TestAddWorkersTeardownOnJoinTimeout proves the deferred teardown fires even
// when a node never joins — the join window must not be left open on failure.
func TestAddWorkersTeardownOnJoinTimeout(t *testing.T) {
	fc := &fakeCluster{
		nodes: addExistingWorkers(), // the new worker never appears → join times out
	}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(2, 16384)

	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	writeIgnitionArtifacts(t, r)
	r.DryRun = false
	r.NodeReadyTimeout = 50 * time.Millisecond

	err := r.AddWorkers(context.Background(), AddOptions{Count: 1})
	if err == nil {
		t.Fatal("want a join-timeout error when the new worker never becomes Ready")
	}
	if fign.teardownCalls != 1 {
		t.Errorf("teardown must fire on a join timeout (deferred): teardown=%d", fign.teardownCalls)
	}
	if fign.configureCalls != 1 {
		t.Errorf("revive must have run before the timeout: configure=%d", fign.configureCalls)
	}
	if fiso.buildCalls != 1 || ftf.applyCalls != 1 {
		t.Errorf("the node must have been built and applied before the join wait: build=%d apply=%d",
			fiso.buildCalls, ftf.applyCalls)
	}
}

// TestAddWorkersTeardownRunsUnderDetachedCtxWhenCancelled proves the security
// fix for the Ctrl-C-during-join-wait window: even when the caller's ctx is
// already cancelled (mirroring SIGINT landing during waitWorkerJoined), the
// deferred teardown must still run to completion under a context that is NOT
// cancelled — otherwise system.IsServiceActive's exec.CommandContext would
// fail to start, StopAndDisableService would wrongly conclude httpd isn't
// running, and the pull-secret-serving ignition server would never be
// stopped or disabled.
func TestAddWorkersTeardownRunsUnderDetachedCtxWhenCancelled(t *testing.T) {
	fc := &fakeCluster{
		nodes: addExistingWorkers(), // the new worker never appears; the join wait observes cancellation
	}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	cfg := addTestConfig(2, 16384)

	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	writeIgnitionArtifacts(t, r)
	r.DryRun = false
	r.NodeReadyTimeout = time.Minute // ctx cancellation must pre-empt this, not the timeout

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate Ctrl-C landing before/during the join wait

	err := r.AddWorkers(ctx, AddOptions{Count: 1})
	if err == nil {
		t.Fatal("want an error when the parent ctx is cancelled during the join wait")
	}

	if fign.configureCalls != 1 {
		t.Errorf("revive must still run before the cancellation is observed: configure=%d", fign.configureCalls)
	}
	if fign.teardownCalls != 1 {
		t.Fatalf("teardown must still fire when ctx is cancelled: teardown=%d", fign.teardownCalls)
	}
	if fign.teardownErrAtCall != nil {
		t.Errorf("teardown must run under a live (non-cancelled) context, got Err()=%v at call time — teardown is not detached from the cancelled parent ctx", fign.teardownErrAtCall)
	}
	if !fign.teardownHadDeadline {
		t.Error("teardown context should carry a bound (timeout) independent of the cancelled parent")
	}
}

// TestAddWorkersResumeSkipsJoinedWorker models a batch interrupted while
// waiting for worker1 to join: worker0 already joined and must be skipped
// whole, worker1 re-runs only its join, and the guards/confirm are skipped.
func TestAddWorkersResumeSkipsJoinedWorker(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "mycluster-worker0", Role: nodetypes.RoleWorker, Ready: true},
			{Name: "mycluster-worker1", Role: nodetypes.RoleWorker, Ready: true},
		},
	}
	ftf := &fakeTF{action: terraform.PlanActionCreate}
	fiso := &fakeISO{}
	fign := &fakeIgnition{}
	// Count never advanced (persist is after the whole batch), so the resumed
	// range is still [0,1].
	cfg := addTestConfig(0, 16384)

	r, _, _ := seedAddRunner(t, fc, ftf, fiso, fign, cfg)
	r.DryRun = false
	seedMarker(t, r, OpAdd, "mycluster-worker1", StepWaitJoin)

	if err := r.AddWorkers(context.Background(), AddOptions{Count: 2}); err != nil {
		t.Fatalf("resumed add: %v", err)
	}

	if fiso.buildCalls != 0 || fiso.uploadCalls != 0 || ftf.applyCalls != 0 {
		t.Errorf("resume at worker1/wait-join must skip worker0 whole and skip worker1's build/upload/apply: build=%d upload=%d apply=%d",
			fiso.buildCalls, fiso.uploadCalls, ftf.applyCalls)
	}
	if fc.approveCalls < 1 {
		t.Errorf("worker1's join must re-run (CSR approval ticks): approve=%d", fc.approveCalls)
	}
	if fc.listNodesCalls != 1 {
		t.Errorf("resume must query ListNodes only for worker1's join (worker0 skipped, guards skipped): listNodes=%d want 1", fc.listNodesCalls)
	}
	if fign.configureCalls != 1 || fign.teardownCalls != 1 {
		t.Errorf("resume must still open/close the join window once: configure=%d teardown=%d", fign.configureCalls, fign.teardownCalls)
	}
	if cfg.Topology.Workers.Count != 2 {
		t.Errorf("completed resume must persist the final worker count: got %d want 2", cfg.Topology.Workers.Count)
	}
}
