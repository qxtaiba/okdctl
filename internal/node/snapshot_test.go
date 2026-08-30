package node

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

const testSnapshotName = "pre-upgrade"

// fakeSnapshotClient records pvesh-backed calls; dry-run asserts none, a real
// run asserts ordering via the shared log.
type fakeSnapshotClient struct {
	log *[]string

	createCalls, rollbackCalls, deleteCalls int

	agentEnabled bool
	agentErr     error

	createErr, rollbackErr, deleteErr error

	snapshots []hostssh.SnapshotInfo

	lastVMID       int
	lastDeleteName string
}

func (f *fakeSnapshotClient) record(event string) {
	if f.log != nil {
		*f.log = append(*f.log, event)
	}
}

func (f *fakeSnapshotClient) CreateSnapshot(_ context.Context, _ *hostssh.RemoteISOParams, vmid int, _, _ string, _ time.Duration) error {
	f.createCalls++
	f.lastVMID = vmid
	f.record("snapshot-create")
	return f.createErr
}

func (f *fakeSnapshotClient) ListSnapshots(_ context.Context, _ *hostssh.RemoteISOParams, vmid int) ([]hostssh.SnapshotInfo, error) {
	f.lastVMID = vmid
	f.record("snapshot-list")
	return f.snapshots, nil
}

func (f *fakeSnapshotClient) RollbackSnapshot(_ context.Context, _ *hostssh.RemoteISOParams, vmid int, _ string, _ time.Duration) error {
	f.rollbackCalls++
	f.lastVMID = vmid
	f.record("snapshot-rollback")
	return f.rollbackErr
}

func (f *fakeSnapshotClient) DeleteSnapshot(_ context.Context, _ *hostssh.RemoteISOParams, vmid int, name string, _ time.Duration) error {
	f.deleteCalls++
	f.lastVMID = vmid
	f.lastDeleteName = name
	f.record("snapshot-delete")
	return f.deleteErr
}

func (f *fakeSnapshotClient) VMAgentEnabled(_ context.Context, _ *hostssh.RemoteISOParams, vmid int) (bool, error) {
	f.lastVMID = vmid
	f.record("vm-agent-enabled")
	if f.agentErr != nil {
		return false, f.agentErr
	}
	return f.agentEnabled, nil
}

func testProxmoxParams() *hostssh.RemoteISOParams {
	return &hostssh.RemoteISOParams{Host: "pve-test", Node: testProxmoxNode}
}

func seedSnapshotRunner(t *testing.T, fc *fakeCluster, fsc *fakeSnapshotClient, cfg *config.Config) *Runner {
	t.Helper()
	r, _, _ := seedRunner(t, fc, &fakeTF{}, cfg)
	r.Proxmox = testProxmoxParams()
	r.Snapshot = fsc
	r.SnapshotTaskTimeout = 5 * time.Second
	return r
}

// seedWorkerSnapshotRunner builds a Runner over a Ready worker0; DryRun stays
// at seedRunner's default (true).
func seedWorkerSnapshotRunner(t *testing.T, fsc *fakeSnapshotClient) (*Runner, *fakeCluster) {
	t.Helper()
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode
	return seedSnapshotRunner(t, fc, fsc, cfg), fc
}

func indicesOf(log []string, event string) []int {
	var out []int
	for i, e := range log {
		if e == event {
			out = append(out, i)
		}
	}
	return out
}

func TestCreateSnapshot_readyNodeCordonsDrainsAndUncordons(t *testing.T) {
	var log []string
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}},
		log:   &log,
	}
	fsc := &fakeSnapshotClient{log: &log}
	cfg := config.DefaultConfig()
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	name, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if name != testSnapshotName {
		t.Errorf("name = %q; want pre-upgrade", name)
	}
	if fc.cordon != 1 || fc.drain != 1 || fc.uncordon != 1 || fsc.createCalls != 1 {
		t.Fatalf("cordon=%d drain=%d uncordon=%d create=%d; want 1 each", fc.cordon, fc.drain, fc.uncordon, fsc.createCalls)
	}

	cordonIdx := indicesOf(log, "cordon")
	drainIdx := indicesOf(log, "drain")
	createIdx := indicesOf(log, "snapshot-create")
	uncordonIdx := indicesOf(log, "uncordon")
	if cordonIdx[0] >= drainIdx[0] || drainIdx[0] >= createIdx[0] || createIdx[0] >= uncordonIdx[0] {
		t.Errorf("call order = %v; want cordon < drain < snapshot-create < uncordon", log)
	}
	if fsc.lastVMID != 6100 {
		t.Errorf("lastVMID = %d; want 6100", fsc.lastVMID)
	}
}

func TestCreateSnapshot_notReadySkipsCordonAndDrain(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: false}}}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 {
		t.Errorf("NotReady node must skip cordon/drain/uncordon: cordon=%d drain=%d uncordon=%d", fc.cordon, fc.drain, fc.uncordon)
	}
	if fsc.createCalls != 1 {
		t.Errorf("createCalls = %d; want 1 (snapshotted directly)", fsc.createCalls)
	}
}

func TestCreateSnapshot_dryRunMakesZeroMutatingCalls(t *testing.T) {
	fsc := &fakeSnapshotClient{}
	r, fc := seedWorkerSnapshotRunner(t, fsc)

	name, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName})
	if err != nil {
		t.Fatalf("dry-run CreateSnapshot: %v", err)
	}
	if name != testSnapshotName {
		t.Errorf("name = %q; want pre-upgrade", name)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 {
		t.Errorf("dry-run mutated the cluster: cordon=%d drain=%d uncordon=%d", fc.cordon, fc.drain, fc.uncordon)
	}
	if fsc.createCalls != 0 {
		t.Errorf("dry-run called CreateSnapshot: createCalls=%d", fsc.createCalls)
	}
}

func TestCreateSnapshot_defaultName(t *testing.T) {
	r, _ := seedWorkerSnapshotRunner(t, &fakeSnapshotClient{})

	name, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if !strings.HasPrefix(name, "okdctl-") {
		t.Errorf("name = %q; want an okdctl- prefixed default", name)
	}
}

// Pins the qemu-agent warning contract: fires when disabled (incl. dry-run) or
// when the probe itself fails, never when the agent is enabled.
func TestCreateSnapshot_crashConsistencyWarning(t *testing.T) {
	tests := []struct {
		name     string
		fsc      *fakeSnapshotClient
		dryRun   bool
		wantWarn bool
	}{
		{name: "disabled dry-run warns", fsc: &fakeSnapshotClient{agentEnabled: false}, dryRun: true, wantWarn: true},
		{name: "enabled no warning", fsc: &fakeSnapshotClient{agentEnabled: true}, dryRun: true, wantWarn: false},
		{name: "probe failure assumes disabled", fsc: &fakeSnapshotClient{agentErr: errors.New("ssh timeout")}, dryRun: true, wantWarn: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := seedWorkerSnapshotRunner(t, tc.fsc)
			r.DryRun = tc.dryRun
			var buf bytes.Buffer
			r.Log = slog.New(slog.NewTextHandler(&buf, nil))

			if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err != nil {
				t.Fatalf("CreateSnapshot: %v", err)
			}
			if got := strings.Contains(buf.String(), "crash-consistent only"); got != tc.wantWarn {
				t.Errorf("crash-consistent-only warning fired=%v, want %v; log:\n%s", got, tc.wantWarn, buf.String())
			}
		})
	}
}

// A leftover OpSnapshot marker would make `okdctl node list` show a phantom in-flight op.
func TestCreateSnapshot_successClearsOpMarker(t *testing.T) {
	r, _ := seedWorkerSnapshotRunner(t, &fakeSnapshotClient{})
	r.DryRun = false

	if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	marker, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil {
		t.Fatalf("ReadOpMarker: %v", err)
	}
	if marker != nil {
		t.Errorf("expected no marker after a clean snapshot; got %+v", marker)
	}
}

// cordonAndDrain is shared with RemoveWorker, so a dying snapshot must stay
// tagged OpSnapshot, not OpRemove.
func TestCreateSnapshot_failureLeavesOpSnapshotMarkerNotOpRemove(t *testing.T) {
	fsc := &fakeSnapshotClient{createErr: errors.New("no space left on device")}
	r, _ := seedWorkerSnapshotRunner(t, fsc)
	r.DryRun = false

	if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err == nil {
		t.Fatal("expected error when the pvesh create call fails")
	}

	marker, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil {
		t.Fatalf("ReadOpMarker: %v", err)
	}
	if marker == nil {
		t.Fatal("expected a marker to remain after a cordoned failure")
	}
	if marker.Op != OpSnapshot {
		t.Errorf("marker.Op = %q; want %q (a later 'okdctl node remove' keys resume on Op equality)", marker.Op, OpSnapshot)
	}
}

// snapshot create is non-resumable: a foreign marker (stranded remove on a
// different node) must refuse unless acknowledged.
func TestCreateSnapshot_refusesForeignMarkerWithoutAck(t *testing.T) {
	fsc := &fakeSnapshotClient{}
	r, fc := seedWorkerSnapshotRunner(t, fsc)
	r.DryRun = false
	seedMarker(t, r, OpRemove, "worker5", StepDrain)

	_, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName})
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError refusing the foreign marker, got %v", err)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 || fsc.createCalls != 0 {
		t.Errorf("refused create must make zero mutation: cordon=%d drain=%d uncordon=%d create=%d",
			fc.cordon, fc.drain, fc.uncordon, fsc.createCalls)
	}

	name, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName, Acknowledge: true})
	if err != nil {
		t.Fatalf("acknowledged create must proceed fresh: %v", err)
	}
	if name != testSnapshotName || fsc.createCalls != 1 {
		t.Errorf("acknowledged create did not run: name=%q createCalls=%d", name, fsc.createCalls)
	}
}

func TestSnapshotOpsRequireProxmox(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
		r, _, _ := seedRunner(t, fc, &fakeTF{}, config.DefaultConfig())

		if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{}); err == nil {
			t.Fatal("expected error when Proxmox is not configured")
		}
	})

	t.Run("rollback", func(t *testing.T) {
		fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
		r, _, _ := seedRunner(t, fc, &fakeTF{}, config.DefaultConfig())

		if err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName, SnapshotRollbackOptions{}); err == nil {
			t.Fatal("expected error when Proxmox is not configured")
		}
	})
}

func TestDeleteSnapshot(t *testing.T) {
	t.Run("real deletes by name", func(t *testing.T) {
		fsc := &fakeSnapshotClient{}
		r, _ := seedWorkerSnapshotRunner(t, fsc)
		r.DryRun = false

		if err := r.DeleteSnapshot(context.Background(), "worker0", testSnapshotName); err != nil {
			t.Fatalf("DeleteSnapshot: %v", err)
		}
		if fsc.deleteCalls != 1 || fsc.lastDeleteName != testSnapshotName {
			t.Errorf("deleteCalls=%d lastDeleteName=%q", fsc.deleteCalls, fsc.lastDeleteName)
		}
	})

	t.Run("dry-run makes zero mutating calls", func(t *testing.T) {
		fsc := &fakeSnapshotClient{}
		r, _ := seedWorkerSnapshotRunner(t, fsc)

		if err := r.DeleteSnapshot(context.Background(), "worker0", testSnapshotName); err != nil {
			t.Fatalf("dry-run DeleteSnapshot: %v", err)
		}
		if fsc.deleteCalls != 0 {
			t.Errorf("dry-run called DeleteSnapshot: deleteCalls=%d", fsc.deleteCalls)
		}
	})
}

// Non-master rollback: no etcd/ceph gates, cordon→rollback→ready-wait→uncordon.
func TestRollbackSnapshot_workerHappyPath(t *testing.T) {
	var log []string
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}},
		log:   &log,
	}
	fsc := &fakeSnapshotClient{log: &log}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	if err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName, SnapshotRollbackOptions{}); err != nil {
		t.Fatalf("RollbackSnapshot: %v", err)
	}
	if fc.cordon != 1 || fc.uncordon != 1 || fsc.rollbackCalls != 1 {
		t.Errorf("cordon=%d uncordon=%d rollback=%d", fc.cordon, fc.uncordon, fsc.rollbackCalls)
	}
	if fc.etcdCalls != 0 || fc.cephCalls != 0 {
		t.Errorf("worker rollback must not gate on etcd/ceph: etcdCalls=%d cephCalls=%d", fc.etcdCalls, fc.cephCalls)
	}
}

func TestRollbackSnapshot_masterGatesHealthPrePostBeforeUncordon(t *testing.T) {
	var log []string
	fc := &fakeCluster{
		nodes:          []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
		log:            &log,
		etcdHealthy:    true,
		cephApplicable: true,
		cephHealthy:    true,
	}
	fsc := &fakeSnapshotClient{log: &log}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	if err := r.RollbackSnapshot(context.Background(), "master0", testSnapshotName, SnapshotRollbackOptions{}); err != nil {
		t.Fatalf("RollbackSnapshot: %v", err)
	}
	if fc.etcdCalls != 2 || fc.cephCalls != 2 {
		t.Fatalf("etcdCalls=%d cephCalls=%d; want 2 each (pre + post)", fc.etcdCalls, fc.cephCalls)
	}

	etcdIdx := indicesOf(log, "etcd")
	cephIdx := indicesOf(log, "ceph")
	cordonIdx := indicesOf(log, "cordon")
	rollbackIdx := indicesOf(log, "snapshot-rollback")
	uncordonIdx := indicesOf(log, "uncordon")

	if etcdIdx[0] >= cephIdx[0] {
		t.Errorf("pre-gate order = %v; want etcd before ceph", log)
	}
	if cephIdx[0] >= cordonIdx[0] || cordonIdx[0] >= rollbackIdx[0] {
		t.Errorf("call order = %v; want pre-gate before cordon before rollback", log)
	}
	if rollbackIdx[0] >= etcdIdx[1] || etcdIdx[1] >= cephIdx[1] {
		t.Errorf("post-gate order = %v; want rollback then etcd then ceph", log)
	}
	if cephIdx[1] >= uncordonIdx[0] {
		t.Errorf("call order = %v; want post-gate before uncordon", log)
	}
}

func TestRollbackSnapshot_postGateFailureLeavesNodeCordoned(t *testing.T) {
	fc := &fakeCluster{
		nodes:                 []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
		etcdHealthy:           true,
		cephApplicable:        true,
		cephHealthy:           true,
		cephUnhealthyFromCall: 2, // pre-gate (call 1) passes; post-gate (call 2+) fails
	}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false
	r.EtcdGateTimeout = 30 * time.Millisecond
	r.CephGateTimeout = 30 * time.Millisecond

	err := r.RollbackSnapshot(context.Background(), "master0", testSnapshotName, SnapshotRollbackOptions{})
	if err == nil {
		t.Fatal("expected error when the post-rollback ceph gate fails")
	}
	if fc.uncordon != 0 {
		t.Errorf("post-gate failure must leave the node cordoned: uncordon=%d", fc.uncordon)
	}
	if fc.cordon != 1 {
		t.Errorf("cordon=%d; want exactly 1 (from cordonAndDrain)", fc.cordon)
	}
	if fsc.rollbackCalls != 1 {
		t.Errorf("rollback must still have run before the post-gate: rollbackCalls=%d", fsc.rollbackCalls)
	}
}

func TestRollbackSnapshot_taskFailureLeavesNodeCordoned(t *testing.T) {
	fsc := &fakeSnapshotClient{rollbackErr: errors.New("no space left on device")}
	r, fc := seedWorkerSnapshotRunner(t, fsc)
	fc.etcdHealthy = true
	r.DryRun = false

	err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName, SnapshotRollbackOptions{})
	if err == nil {
		t.Fatal("expected error when the rollback task fails")
	}
	if fc.uncordon != 0 {
		t.Errorf("task failure must leave the node cordoned: uncordon=%d", fc.uncordon)
	}
	if !strings.Contains(err.Error(), "left cordoned") {
		t.Errorf("err = %q; want it to name the cordoned state", err.Error())
	}
}

// Mirrors compact's dry-run-never-blocks contract.
func TestRollbackSnapshot_dryRunMakesZeroMutatingCalls(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
		etcdHealthy: false, // degraded: the real gate would block up to EtcdGateTimeout
	}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	// seedRunner defaults DryRun to true.

	if err := r.RollbackSnapshot(context.Background(), "master0", testSnapshotName, SnapshotRollbackOptions{}); err != nil {
		t.Fatalf("dry-run RollbackSnapshot: %v", err)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 || fc.etcdCalls != 0 || fc.cephCalls != 0 {
		t.Errorf("dry-run made unexpected calls: cordon=%d drain=%d uncordon=%d etcd=%d ceph=%d",
			fc.cordon, fc.drain, fc.uncordon, fc.etcdCalls, fc.cephCalls)
	}
	if fsc.rollbackCalls != 0 {
		t.Errorf("dry-run called RollbackSnapshot: rollbackCalls=%d", fsc.rollbackCalls)
	}
}

func TestRollbackSnapshot_successClearsOpMarker(t *testing.T) {
	r, _ := seedWorkerSnapshotRunner(t, &fakeSnapshotClient{})
	r.DryRun = false

	if err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName, SnapshotRollbackOptions{}); err != nil {
		t.Fatalf("RollbackSnapshot: %v", err)
	}

	marker, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil {
		t.Fatalf("ReadOpMarker: %v", err)
	}
	if marker != nil {
		t.Errorf("expected no marker after a clean rollback; got %+v", marker)
	}
}

// A failed final uncordon must surface as the op's error, never a warning behind nil.
func TestRollbackSnapshot_finalUncordonFailureSurfacesAsError(t *testing.T) {
	r, fc := seedWorkerSnapshotRunner(t, &fakeSnapshotClient{})
	fc.uncordonErr = errors.New("connection refused")
	r.DryRun = false

	err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName, SnapshotRollbackOptions{})
	if err == nil {
		t.Fatal("expected error when the final uncordon fails")
	}
	if !strings.Contains(err.Error(), "left cordoned") {
		t.Errorf("err = %q; want it to name the cordoned state", err.Error())
	}
	if fc.uncordon != 1 {
		t.Errorf("uncordon calls = %d; want 1 (attempted, even though it failed)", fc.uncordon)
	}

	marker, merr := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if merr != nil {
		t.Fatalf("ReadOpMarker: %v", merr)
	}
	if marker == nil || marker.Op != OpSnapshot {
		t.Errorf("marker = %+v; want a persisted OpSnapshot marker (not cleared on final uncordon failure)", marker)
	}
}

func TestRollbackSnapshot_refusesForeignMarkerWithoutAck(t *testing.T) {
	fsc := &fakeSnapshotClient{}
	r, fc := seedWorkerSnapshotRunner(t, fsc)
	r.DryRun = false
	seedMarker(t, r, OpRemove, "worker5", StepDrain)

	err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName, SnapshotRollbackOptions{})
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError refusing the foreign marker, got %v", err)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 || fsc.rollbackCalls != 0 {
		t.Errorf("refused rollback must make zero mutation: cordon=%d drain=%d uncordon=%d rollback=%d",
			fc.cordon, fc.drain, fc.uncordon, fsc.rollbackCalls)
	}

	if err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName, SnapshotRollbackOptions{Acknowledge: true}); err != nil {
		t.Fatalf("acknowledged rollback must proceed fresh: %v", err)
	}
	if fsc.rollbackCalls != 1 {
		t.Errorf("acknowledged rollback did not run: rollbackCalls=%d", fsc.rollbackCalls)
	}
}

// --skip-drain create writes no marker of its own, so the guard must delete the
// acknowledged one or it resurfaces.
func TestCreateSnapshot_acknowledgeConsumesForeignMarker(t *testing.T) {
	r, _ := seedWorkerSnapshotRunner(t, &fakeSnapshotClient{})
	r.DryRun = false
	seedMarker(t, r, OpRemove, "worker5", StepDrain)

	if _, err := r.CreateSnapshot(context.Background(), "worker0",
		SnapshotCreateOptions{Name: testSnapshotName, SkipDrain: true, Acknowledge: true}); err != nil {
		t.Fatalf("acknowledged --skip-drain create must proceed: %v", err)
	}

	marker, err := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if err != nil {
		t.Fatalf("re-read marker: %v", err)
	}
	if marker != nil {
		t.Fatalf("acknowledged marker must be consumed, still present: %+v", marker)
	}

	// The next op must run clean with no acknowledgement needed.
	if _, err := r.CreateSnapshot(context.Background(), "worker0",
		SnapshotCreateOptions{Name: testSnapshotName, SkipDrain: true}); err != nil {
		t.Fatalf("follow-up op must not re-refuse a consumed marker: %v", err)
	}
}
