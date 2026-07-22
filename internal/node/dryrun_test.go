package node

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

const testProxmoxNode = "pve"

// fakeCluster records the mutating calls a node op makes so a dry-run can be
// asserted to make none. The …Nodes slices additionally record which node
// each call targeted, and listNodesCalls/podsForSelectorCalls count the
// read-only guard queries, so a resumed op can be proven to have skipped
// guards/validation rather than merely having them no-op.
type fakeCluster struct {
	nodes          []cluster.NodeDetail
	osdPods        []cluster.PodPlacement
	routerPods     []cluster.PodPlacement
	cordon         int
	drain          int
	uncordon       int
	deleteNode     int
	setSched       int
	applied        int
	schedulable    bool
	etcdHealthy    bool
	cephApplicable bool
	cephHealthy    bool
	// drainFailsAtCall makes the Nth Drain call (1-based) fail; 0 never fails.
	drainFailsAtCall int

	approveCalls   int
	approveCount   int
	approveErr     error
	signerNotAfter time.Time
	signerErr      error

	listNodesCalls       int
	podsForSelectorCalls int
	cordonedNodes        []string
	drainedNodes         []string
	uncordonedNodes      []string
	// readyAtCall, when >0, forces ListNodes to report every node not-Ready
	// until the Nth call (1-based): drives a start test's readiness poll from
	// not-ready to Ready without a live API.
	readyAtCall int
	// listErr, when set, makes ListNodes fail — the API unreachable while a
	// just-started control plane is still coming up.
	listErr error
}

func (f *fakeCluster) ListNodes(context.Context) ([]cluster.NodeDetail, error) {
	f.listNodesCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.readyAtCall > 0 {
		ready := f.listNodesCalls >= f.readyAtCall
		out := make([]cluster.NodeDetail, len(f.nodes))
		copy(out, f.nodes)
		for i := range out {
			out[i].Ready = ready
		}
		return out, nil
	}
	return f.nodes, nil
}

func (f *fakeCluster) Cordon(_ context.Context, node string) error {
	f.cordon++
	f.cordonedNodes = append(f.cordonedNodes, node)
	return nil
}

func (f *fakeCluster) Uncordon(_ context.Context, node string) error {
	f.uncordon++
	f.uncordonedNodes = append(f.uncordonedNodes, node)
	return nil
}

func (f *fakeCluster) Drain(_ context.Context, node string, _ cluster.DrainOptions) error {
	f.drain++
	f.drainedNodes = append(f.drainedNodes, node)
	if f.drainFailsAtCall != 0 && f.drain == f.drainFailsAtCall {
		return errors.New("drain timed out")
	}
	return nil
}

// DeleteNode drops the node from the fake's node list so a subsequent ListNodes
// reflects the removal, letting a multi-worker compact loop run realistically.
func (f *fakeCluster) DeleteNode(_ context.Context, name string) error {
	f.deleteNode++
	kept := f.nodes[:0]
	for _, n := range f.nodes {
		if n.Name != name {
			kept = append(kept, n)
		}
	}
	f.nodes = kept
	return nil
}

func (f *fakeCluster) EtcdHealthy(context.Context) (cluster.EtcdHealth, error) {
	return cluster.EtcdHealth{Healthy: f.etcdHealthy}, nil
}

func (f *fakeCluster) CephHealthy(context.Context) (cluster.CephHealth, error) {
	return cluster.CephHealth{Applicable: f.cephApplicable, Healthy: f.cephHealthy}, nil
}

func (f *fakeCluster) MastersSchedulable(context.Context) (bool, error) { return f.schedulable, nil }

func (f *fakeCluster) SetMastersSchedulable(context.Context, bool) error {
	f.setSched++
	return nil
}

func (f *fakeCluster) PodsForSelector(_ context.Context, namespace, selector string) ([]cluster.PodPlacement, error) {
	f.podsForSelectorCalls++
	if selector == "app=rook-ceph-osd" {
		return f.osdPods, nil
	}
	if namespace == "openshift-ingress" {
		return f.routerPods, nil
	}
	return nil, nil
}
func (f *fakeCluster) Apply(context.Context, []byte) error { f.applied++; return nil }

func (f *fakeCluster) ApprovePendingCSRs(context.Context) (int, error) {
	f.approveCalls++
	if f.approveErr != nil {
		return 0, f.approveErr
	}
	return f.approveCount, nil
}

func (f *fakeCluster) SignerNotAfter(context.Context) (time.Time, error) {
	if f.signerErr != nil {
		return time.Time{}, f.signerErr
	}
	return f.signerNotAfter, nil
}

// fakeTF records plan/apply calls and echoes a single in-place-or-delete change
// for whatever the plan targeted, so the plan gate is exercised without running
// terraform.
type fakeTF struct {
	planCalls  int
	applyCalls int
	snapshots  int
	lastVars   map[string]string
	lastTarget string
	action     terraform.PlanAction
	// emptyPlan makes ShowPlanChanges report no changes, simulating a
	// resumed re-run where the apply already landed.
	emptyPlan bool
	// emptyForAddress makes ShowPlanChanges report an empty plan only for the
	// listed targeted addresses, on top of the emptyPlan bool — used to model
	// a role roll where some nodes in the same fakeTF have already landed and
	// others have not.
	emptyForAddress map[string]bool
	// stateAbsent flips StateHasResource to report the target address as
	// absent from state; zero value (false) keeps the common "still
	// present" default so tests that never reach the empty-plan branch see
	// no behavior change.
	stateAbsent bool
	stateCalls  int
}

func (f *fakeTF) Init(context.Context) error { return nil }
func (f *fakeTF) Plan(_ context.Context, opts terraform.PlanOptions) error {
	f.planCalls++
	f.lastVars = opts.Vars
	if len(opts.Targets) > 0 {
		f.lastTarget = opts.Targets[0]
	}
	return nil
}

func (f *fakeTF) ShowPlanChanges(context.Context, string) ([]terraform.ResourceChange, error) {
	if f.emptyPlan || f.emptyForAddress[f.lastTarget] {
		return nil, nil
	}
	return []terraform.ResourceChange{{Address: f.lastTarget, Action: f.action}}, nil
}
func (f *fakeTF) SnapshotState(context.Context) (string, error) { f.snapshots++; return "", nil }

func (f *fakeTF) StateHasResource(context.Context, string) (bool, error) {
	f.stateCalls++
	return !f.stateAbsent, nil
}

func (f *fakeTF) Apply(context.Context, terraform.ApplyOptions) error {
	f.applyCalls++
	return nil
}
func (f *fakeTF) WithLockHint(err error) error { return err }

// fakePower records power-cycle calls so a resize can be asserted to realize
// the change via the hypervisor. err simulates a PowerCycleVM API failure;
// shutdownErr/startErr do the same for ShutdownVM/StartVM. cycledVMIDs records
// every PowerCycleVM call (not just the last) so a multi-node role roll can
// assert exactly which VMs were power-cycled.
type fakePower struct {
	calls       int
	lastNode    string
	lastVMID    int
	cycledVMIDs []int
	err         error

	shutdownCalls int
	shutdownErr   error
	// shutdownFailsAtCall makes the Nth ShutdownVM call (1-based) fail; 0 never fails.
	shutdownFailsAtCall int
	// shutdownOrder records each ShutdownVM call's vmid in call order, so a
	// test can assert workers-before-masters sequencing.
	shutdownOrder []int

	startCalls int
	startErr   error
	// startOrder records each StartVM call's vmid in call order, so a test can
	// assert masters-before-workers sequencing.
	startOrder []int

	running bool
}

func (f *fakePower) PowerCycleVM(_ context.Context, node string, vmid int) error {
	f.calls++
	f.lastNode = node
	f.lastVMID = vmid
	f.cycledVMIDs = append(f.cycledVMIDs, vmid)
	return f.err
}

func (f *fakePower) ShutdownVM(_ context.Context, node string, vmid int) error {
	f.shutdownCalls++
	f.lastNode = node
	f.lastVMID = vmid
	f.shutdownOrder = append(f.shutdownOrder, vmid)
	if f.shutdownFailsAtCall != 0 && f.shutdownCalls == f.shutdownFailsAtCall {
		return errors.New("shutdown failed")
	}
	return f.shutdownErr
}

func (f *fakePower) StartVM(_ context.Context, node string, vmid int) error {
	f.startCalls++
	f.lastNode = node
	f.lastVMID = vmid
	f.startOrder = append(f.startOrder, vmid)
	return f.startErr
}

func (f *fakePower) VMRunning(context.Context, string, int) (bool, error) {
	return f.running, nil
}

func seedRunner(t *testing.T, fc *fakeCluster, ftf *fakeTF, cfg *config.Config) (r *Runner, tfvarsPath, cfgPath string) {
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
		Cluster:          fc,
		TF:               ftf,
		Cfg:              cfg,
		ConfigPath:       cfgPath,
		WorkDir:          dir,
		EnvDir:           dir,
		RunID:            "test-run",
		DryRun:           true,
		Log:              logutil.NopLogger,
		NodeReadyTimeout: 5 * time.Second,
		EtcdGateTimeout:  5 * time.Second,
		CephGateTimeout:  5 * time.Second,
	}
	return r, tfvarsPath, cfgPath
}

func assertUnchanged(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s was modified during dry-run:\n got %q\nwant %q", path, got, want)
	}
}

func TestResizeDryRunMakesNoMutation(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)

	if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576}); err != nil {
		t.Fatalf("dry-run resize: %v", err)
	}

	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 {
		t.Errorf("dry-run resize mutated the cluster: cordon=%d drain=%d uncordon=%d", fc.cordon, fc.drain, fc.uncordon)
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("dry-run resize applied terraform: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
	if ftf.planCalls == 0 {
		t.Error("dry-run resize produced no plan preview")
	}
	if ftf.lastVars["master_memory_mb"] != "24576" {
		t.Errorf("dry-run plan did not carry the sizing override: vars=%v", ftf.lastVars)
	}
	if ftf.lastVars["bootstrap_enabled"] != "false" || ftf.lastVars["start_workers_immediately"] != "true" {
		t.Errorf("plan missing post-deploy invariants (bootstrap_enabled=false, start_workers_immediately=true): vars=%v", ftf.lastVars)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
	if _, err := os.Stat(filepath.Join(r.WorkDir, OpMarkerFileName)); !os.IsNotExist(err) {
		t.Error("dry-run resize wrote an op marker")
	}
}

func TestResizeMemoryBudgetEnforcedWhenProbeFeedsValues(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, _, _ := seedRunner(t, fc, ftf, cfg)

	// Host nearly full: growing a master by ~28 GiB must be refused by the guard.
	err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{
		MemoryMB:         40960,
		HostTotalMiB:     96000,
		HostAllocatedMiB: 93000,
	})
	if err == nil {
		t.Fatal("want memory-budget refusal when the probe reports the host is full")
	}
	if ftf.planCalls != 0 {
		t.Errorf("budget refusal must precede any plan; planCalls=%d", ftf.planCalls)
	}
}

func TestResizeMemoryBudgetDegradesWhenProbeAbsent(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, _, _ := seedRunner(t, fc, ftf, cfg)

	// Zero host budget models a failed/absent probe: the guard must warn and
	// continue, not hard-fail the resize.
	err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{
		MemoryMB:         40960,
		HostTotalMiB:     0,
		HostAllocatedMiB: 0,
	})
	if err != nil {
		t.Fatalf("absent probe must degrade to a warning, not fail: %v", err)
	}
}

func TestResizeOneNodePowerCyclesAndRealizes(t *testing.T) {
	fc := &fakeCluster{
		nodes:          []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}},
		cephApplicable: true,
		cephHealthy:    true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	fp := &fakePower{}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = fp

	if err := r.resizeOneNode(context.Background(), resizeTarget{name: "worker0", index: 0}, nodetypes.RoleWorker, map[string]string{"worker_memory_mb": "16384"}, nil); err != nil {
		t.Fatalf("resizeOneNode: %v", err)
	}
	if fp.calls != 1 {
		t.Fatalf("expected exactly one power-cycle, got %d", fp.calls)
	}
	if fp.lastVMID != 6100 || fp.lastNode != testProxmoxNode {
		t.Fatalf("power-cycle addressed wrong vm: node=%q vmid=%d (want pve/6100)", fp.lastNode, fp.lastVMID)
	}
	if fc.cordon != 1 || fc.drain != 1 || fc.uncordon != 1 {
		t.Errorf("expected cordon/drain/uncordon once each: cordon=%d drain=%d uncordon=%d", fc.cordon, fc.drain, fc.uncordon)
	}
}

func TestResizeOneNodePowerCycleFailureLeavesCordoned(t *testing.T) {
	fc := &fakeCluster{
		nodes:          []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}},
		cephApplicable: true,
		cephHealthy:    true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	fp := &fakePower{err: errors.New("proxmox api unreachable")}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = fp

	err := r.resizeOneNode(context.Background(), resizeTarget{name: "worker0", index: 0}, nodetypes.RoleWorker, map[string]string{"worker_memory_mb": "16384"}, nil)
	if err == nil {
		t.Fatal("expected error when power-cycle fails")
	}
	if fc.uncordon != 0 {
		t.Errorf("node must be left cordoned on power-cycle failure; uncordon=%d", fc.uncordon)
	}
	if fp.calls != 1 {
		t.Errorf("expected one power-cycle attempt, got %d", fp.calls)
	}
}

func TestResizeRefusesWithoutPowerCycler(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.MemoryMB = 12288

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = nil

	err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleWorker}, ResizeOptions{MemoryMB: 16384})
	if err == nil {
		t.Fatal("expected refusal when no power-cycler is wired")
	}
	if fc.cordon != 0 || fc.drain != 0 {
		t.Errorf("refusal must precede any disruption: cordon=%d drain=%d", fc.cordon, fc.drain)
	}
}

func TestResizeCPUOnlyKeepsMemoryUnchanged(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288
	cfg.Topology.ControlPlane.CPU = 4

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)

	// Host nearly full: a CPU-only resize must resolve memory to its current
	// value (zero delta) so the budget guard skips cleanly instead of tripping
	// on a spuriously huge delta computed against an unset MemoryMB.
	err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{
		CPU:              8,
		HostTotalMiB:     96000,
		HostAllocatedMiB: 93000,
	})
	if err != nil {
		t.Fatalf("cpu-only resize: %v", err)
	}

	if ftf.lastVars["master_memory_mb"] != "12288" {
		t.Errorf("cpu-only resize must plan memory at its current value: vars=%v", ftf.lastVars)
	}
	if ftf.lastVars["master_cpu_cores"] != "8" {
		t.Errorf("cpu-only resize did not carry the cpu override: vars=%v", ftf.lastVars)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 {
		t.Errorf("dry-run resize mutated the cluster: cordon=%d drain=%d uncordon=%d", fc.cordon, fc.drain, fc.uncordon)
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("dry-run resize applied terraform: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
}

func TestResizeRequiresMemoryOrCPU(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, _, _ := seedRunner(t, fc, ftf, cfg)

	err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{})
	if err == nil {
		t.Fatal("want error when neither MemoryMB nor CPU is set")
	}
	if ftf.planCalls != 0 || fc.cordon != 0 {
		t.Errorf("usage error must precede any plan or mutation: planCalls=%d cordon=%d", ftf.planCalls, fc.cordon)
	}
}

func TestRemoveDryRunPreviewIsTruthfulAndInert(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: "worker2", Role: nodetypes.RoleWorker},
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 3

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)

	if err := r.RemoveWorker(context.Background(), "worker2", RemoveOptions{}); err != nil {
		t.Fatalf("dry-run remove: %v", err)
	}

	if fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 {
		t.Errorf("dry-run remove mutated the cluster: cordon=%d drain=%d deleteNode=%d", fc.cordon, fc.drain, fc.deleteNode)
	}
	if ftf.applyCalls != 0 {
		t.Errorf("dry-run remove applied terraform: apply=%d", ftf.applyCalls)
	}
	// #4 fidelity: the preview must feed worker_count=2 so the delete plan is
	// real rather than a spuriously-empty no-op.
	if ftf.lastVars["worker_count"] != "2" {
		t.Errorf("dry-run remove plan did not carry worker_count override: vars=%v", ftf.lastVars)
	}
	if ftf.lastVars["bootstrap_enabled"] != "false" || ftf.lastVars["start_workers_immediately"] != "true" {
		t.Errorf("remove plan missing post-deploy invariants: vars=%v", ftf.lastVars)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
}

func compactNodes() []cluster.NodeDetail {
	return []cluster.NodeDetail{
		{Name: "worker0", Role: nodetypes.RoleWorker},
		{Name: "worker1", Role: nodetypes.RoleWorker},
		{Name: "master0", Role: nodetypes.RoleMaster},
	}
}

func TestCompactDryRunPreviewsEveryWorkerAndMakesNoMutation(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: "worker2", Role: nodetypes.RoleWorker},
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 3

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)

	if err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2}); err != nil {
		t.Fatalf("dry-run compact: %v", err)
	}

	// Zero mutation: no control-plane change, no ingress apply, no cluster ops.
	if fc.setSched != 0 || fc.applied != 0 {
		t.Errorf("dry-run compact mutated the control plane: setSched=%d applied=%d", fc.setSched, fc.applied)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 {
		t.Errorf("dry-run compact mutated the cluster: cordon=%d drain=%d deleteNode=%d", fc.cordon, fc.drain, fc.deleteNode)
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("dry-run compact applied terraform: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
	// One real delete plan gate per worker.
	if ftf.planCalls != 3 {
		t.Errorf("dry-run compact must plan-gate every worker: planCalls=%d want 3", ftf.planCalls)
	}
	// Last gate is worker0: worker[0] leaves when worker_count drops to 0. The
	// bootstrap/start-workers invariants are asserted by the remove/resize
	// dry-run tests; every gate flows through the same nodeOpPlanVars helper.
	if ftf.lastVars["worker_count"] != "0" {
		t.Errorf("compact plan gate did not decrement worker_count per worker: vars=%v", ftf.lastVars)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
	if _, err := os.Stat(filepath.Join(r.WorkDir, OpMarkerFileName)); !os.IsNotExist(err) {
		t.Error("dry-run compact wrote an op marker")
	}
}

// TestCompactDryRunAgainstDegradedEtcdStillPreviews locks the fix that makes
// the pre-flight etcd gate non-blocking under --dry-run: a degraded quorum must
// not hang the preview, and the verdict is surfaced as a line rather than a
// failure.
func TestCompactDryRunAgainstDegradedEtcdStillPreviews(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
		etcdHealthy: false, // degraded: the real gate would block up to 10m
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 2

	dir := t.TempDir()
	var buf bytes.Buffer
	r := &Runner{
		Cluster:    fc,
		TF:         ftf,
		Cfg:        cfg,
		ConfigPath: filepath.Join(dir, "okdctl.yaml"),
		WorkDir:    dir,
		EnvDir:     dir,
		RunID:      "test-run",
		DryRun:     true,
		Log:        slog.New(slog.NewTextHandler(&buf, nil)),
	}

	if err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2}); err != nil {
		t.Fatalf("dry-run compact against degraded etcd must not fail: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "compact plan") {
		t.Errorf("preview did not print against degraded etcd:\n%s", out)
	}
	if !strings.Contains(out, "etcd: UNHEALTHY") || !strings.Contains(out, "wait up to 10m") {
		t.Errorf("preview missing the etcd verdict line:\n%s", out)
	}

	// Zero mutation despite the degraded quorum.
	if fc.setSched != 0 || fc.applied != 0 || fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 {
		t.Errorf("dry-run compact mutated the cluster: setSched=%d applied=%d cordon=%d drain=%d delete=%d",
			fc.setSched, fc.applied, fc.cordon, fc.drain, fc.deleteNode)
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("dry-run compact applied terraform: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
}

func TestCompactPreflightsStorageGuardBeforeControlPlaneMutation(t *testing.T) {
	fc := &fakeCluster{
		nodes:       compactNodes(),
		schedulable: true,
		etcdHealthy: true,
		// An OSD on the first worker to be removed must block the whole compact.
		osdPods: []cluster.PodPlacement{{Name: "osd-0", Namespace: "rook-ceph", NodeName: "worker1"}},
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 2

	r, tfvars, cfgPath := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false

	err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2})
	if err == nil {
		t.Fatal("want storage-guard refusal before any mutation")
	}
	if !strings.Contains(err.Error(), "rook-ceph OSD") {
		t.Errorf("refusal should name the storage guard: %v", err)
	}
	// The refusal must precede SetMastersSchedulable and the ingress apply.
	if fc.setSched != 0 || fc.applied != 0 {
		t.Errorf("guard refusal mutated the control plane: setSched=%d applied=%d", fc.setSched, fc.applied)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 || ftf.applyCalls != 0 {
		t.Errorf("guard refusal mutated cluster/terraform: cordon=%d drain=%d delete=%d apply=%d", fc.cordon, fc.drain, fc.deleteNode, ftf.applyCalls)
	}
	assertUnchanged(t, tfvars, "SENTINEL_TFVARS\n")
	assertUnchanged(t, cfgPath, "SENTINEL_CONFIG\n")
}

func TestCompactHybridStateReportedOnMidLoopFailure(t *testing.T) {
	fc := &fakeCluster{
		nodes:            compactNodes(),
		schedulable:      true,
		etcdHealthy:      true,
		drainFailsAtCall: 2, // first worker drains; the second's drain fails
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 2

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false

	err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2})
	if err == nil {
		t.Fatal("want a hybrid-state error when a mid-loop removal fails")
	}
	msg := err.Error()
	if !strings.Contains(msg, "1 of 2") || !strings.Contains(msg, "re-run") {
		t.Errorf("hybrid error must report how many removed and how to proceed: %v", err)
	}
	// The failure is post-mutation: the control plane was already made schedulable
	// and the first worker was removed before the second's drain failed.
	if fc.setSched != 1 || fc.applied != 1 {
		t.Errorf("compact should have mutated the control plane before the failure: setSched=%d applied=%d", fc.setSched, fc.applied)
	}
	if fc.deleteNode != 1 {
		t.Errorf("exactly one worker should have been removed before the failure: deleteNode=%d", fc.deleteNode)
	}
}

const testWorkerAddress = "module.okd_cluster.proxmox_virtual_environment_vm.worker[2]"

func TestTargetedApplyAlreadyAtTargetSkipsApply(t *testing.T) {
	fc := &fakeCluster{}
	ftf := &fakeTF{action: terraform.PlanActionDelete, emptyPlan: true, stateAbsent: true}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false

	if err := r.targetedApply(context.Background(), testWorkerAddress, terraform.PlanActionDelete, nil); err != nil {
		t.Fatalf("targetedApply: %v", err)
	}
	if ftf.stateCalls != 1 {
		t.Errorf("expected exactly one StateHasResource probe on the empty-plan path, got %d", ftf.stateCalls)
	}
	if ftf.applyCalls != 0 || ftf.snapshots != 0 {
		t.Errorf("already-at-target must skip apply: apply=%d snapshot=%d", ftf.applyCalls, ftf.snapshots)
	}
}

func TestTargetedApplyEmptyPlanStillAwayFromTargetIsGateFailure(t *testing.T) {
	fc := &fakeCluster{}
	ftf := &fakeTF{action: terraform.PlanActionDelete, emptyPlan: true, stateAbsent: false}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false

	err := r.targetedApply(context.Background(), testWorkerAddress, terraform.PlanActionDelete, nil)
	if err == nil {
		t.Fatal("want a gate-refusal error when the empty plan does not mean already-at-target")
	}
	if ftf.applyCalls != 0 {
		t.Errorf("gate failure must not apply: apply=%d", ftf.applyCalls)
	}
}

func TestTargetedApplyHappyPathNeverProbesState(t *testing.T) {
	fc := &fakeCluster{}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false

	if err := r.targetedApply(context.Background(), testWorkerAddress, terraform.PlanActionDelete, nil); err != nil {
		t.Fatalf("targetedApply: %v", err)
	}
	if ftf.stateCalls != 0 {
		t.Errorf("happy path must not pay the state-list probe: stateCalls=%d", ftf.stateCalls)
	}
	if ftf.applyCalls != 1 {
		t.Errorf("expected exactly one apply, got %d", ftf.applyCalls)
	}
}
