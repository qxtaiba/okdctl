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
	"github.com/qxtaiba/okdctl/internal/distribution/okd/provision"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/workspace"
)

// fakeISO records BuildCustomISOs/UploadCustomISOsToProxmox calls; events, when
// set, records ordering shared with the other fakes.
type fakeISO struct {
	buildCalls  int
	uploadCalls int
	events      *[]string
}

func (f *fakeISO) BuildCustomISOs(context.Context, *config.Config, provision.Options) error {
	f.buildCalls++
	if f.events != nil {
		*f.events = append(*f.events, "build")
	}
	return nil
}

func (f *fakeISO) UploadCustomISOsToProxmox(context.Context, *config.Config, provision.Options) error {
	f.uploadCalls++
	if f.events != nil {
		*f.events = append(*f.events, "upload")
	}
	return nil
}

// fakeIgnition records Revive/TeardownIgnitionServer calls; teardownErrAtCall/teardownHadDeadline
// snapshot ctx synchronously inside the call since the caller's deferred
// cancel() may fire right after return.
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
	// Keeps DefaultConfig's full Proxmox block so persistTopology can render terraform.tfvars.
	cfg.Provider.Proxmox.ISOStorage = "iso-store"
	return cfg
}

// seedAddRunner seeds sentinel tfvars/config files so a dry-run's zero-mutation
// contract can be checked.
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
		projectRoot: dir,
		workDir:     filepath.Join(dir, "okd-install"),
		envDir:      dir,
		RunID:       "test-run",
		DryRun:      true,
		Log:         logutil.NopLogger,
	}
	return r, tfvarsPath, cfgPath
}

type addTestHarness struct {
	r               *Runner
	ftf             *fakeTF
	fiso            *fakeISO
	fign            *fakeIgnition
	tfvars, cfgPath string
}

// seedAddTest wires a create-shaped terraform fake; the ordering test builds
// its own event-recording fakes instead.
func seedAddTest(t *testing.T, fc *fakeCluster, cfg *config.Config) addTestHarness {
	t.Helper()
	h := addTestHarness{
		ftf:  &fakeTF{action: terraform.PlanActionCreate},
		fiso: &fakeISO{},
		fign: &fakeIgnition{},
	}
	h.r, h.tfvars, h.cfgPath = seedAddRunner(t, fc, h.ftf, h.fiso, h.fign, cfg)
	return h
}

func writeIgnitionArtifacts(t *testing.T, r *Runner) {
	t.Helper()
	clusterDir := workspace.ClusterConfigDir(r.workDir)
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clusterDir, "worker.ign"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	certPath, _ := provision.IgnitionCertPaths(r.projectRoot)
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
	h := seedAddTest(t, fc, addTestConfig(2, 16384))
	r, ftf, fiso, fign, tfvars, cfgPath := h.r, h.ftf, h.fiso, h.fign, h.tfvars, h.cfgPath
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
	// worker_count and worker_isos must widen together, or the module's length assertion trips.
	if ftf.lastVars["worker_count"] != "4" {
		t.Errorf("dry-run add plan did not widen worker_count: vars=%v", ftf.lastVars)
	}
	wantISOs := `["iso-store:iso/worker0.iso", "iso-store:iso/worker1.iso", "iso-store:iso/worker2.iso", "iso-store:iso/worker3.iso"]`
	if ftf.lastVars["worker_isos"] != wantISOs {
		t.Errorf("dry-run add plan did not widen worker_isos in lockstep: got %q want %q", ftf.lastVars["worker_isos"], wantISOs)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
	if _, err := os.Stat(filepath.Join(r.workDir, OpMarkerFileName)); !os.IsNotExist(err) {
		t.Error("dry-run add wrote an op marker")
	}
}

func TestAddWorkersMemoryBudgetRejected(t *testing.T) {
	fc := &fakeCluster{nodes: addWorkerNodes(2)}
	h := seedAddTest(t, fc, addTestConfig(2, 40960))
	r, ftf, fiso, fign := h.r, h.ftf, h.fiso, h.fign
	writeIgnitionArtifacts(t, r)

	// Host nearly full: must be refused.
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
	// No writeIgnitionArtifacts call models `cleanup --kind web-only` having removed worker.ign/cert.
	h := seedAddTest(t, fc, addTestConfig(1, 16384))
	r, ftf, fiso := h.r, h.ftf, h.fiso

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
			r := seedAddTest(t, fc, addTestConfig(tt.startCount, 16384)).r
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

// addExistingWorkers builds a pre-add worker roster whose names the join wait can match.
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

func TestAddWorkersTeardownOnJoinTimeout(t *testing.T) {
	// The new worker never appears → join times out.
	fc := &fakeCluster{nodes: addExistingWorkers()}
	h := seedAddTest(t, fc, addTestConfig(2, 16384))
	r, ftf, fiso, fign := h.r, h.ftf, h.fiso, h.fign
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

// A cancelled ctx must not reach teardown, or StopAndDisableService would
// wrongly conclude httpd isn't running and leave the pull-secret server up.
func TestAddWorkersTeardownRunsUnderDetachedCtxWhenCancelled(t *testing.T) {
	fc := &fakeCluster{nodes: addExistingWorkers()}
	h := seedAddTest(t, fc, addTestConfig(2, 16384))
	r, fign := h.r, h.fign
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

func TestAddWorkersResumeSkipsJoinedWorker(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "mycluster-worker0", Role: nodetypes.RoleWorker, Ready: true},
			{Name: "mycluster-worker1", Role: nodetypes.RoleWorker, Ready: true},
		},
	}
	// Count never advanced (persist is after the whole batch), so the resumed range is still [0,1].
	h := seedAddTest(t, fc, addTestConfig(0, 16384))
	r, ftf, fiso, fign := h.r, h.ftf, h.fiso, h.fign
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
	if r.Cfg.Topology.Workers.Count != 2 {
		t.Errorf("completed resume must persist the final worker count: got %d want 2", r.Cfg.Topology.Workers.Count)
	}
}
