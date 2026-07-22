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
	"github.com/qxtaiba/okdctl/internal/infrastructure/proxmox/hostssh"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

const testSnapshotName = "pre-upgrade"

// fakeSnapshotClient records the pvesh-backed calls a snapshot op makes, so a
// dry-run can be asserted to make none and a real run can be asserted to make
// them in the right order (via the shared log, when set).
type fakeSnapshotClient struct {
	log *[]string

	createCalls, listCalls, rollbackCalls, deleteCalls, agentCalls int

	agentEnabled bool
	agentErr     error

	createErr, listErr, rollbackErr, deleteErr error

	snapshots []hostssh.SnapshotInfo

	lastVMID                                         int
	lastCreateName, lastRollbackName, lastDeleteName string
}

func (f *fakeSnapshotClient) record(event string) {
	if f.log != nil {
		*f.log = append(*f.log, event)
	}
}

func (f *fakeSnapshotClient) CreateSnapshot(_ context.Context, _ *hostssh.RemoteISOParams, vmid int, name, _ string, _ time.Duration) error {
	f.createCalls++
	f.lastVMID = vmid
	f.lastCreateName = name
	f.record("snapshot-create")
	return f.createErr
}

func (f *fakeSnapshotClient) ListSnapshots(_ context.Context, _ *hostssh.RemoteISOParams, vmid int) ([]hostssh.SnapshotInfo, error) {
	f.listCalls++
	f.lastVMID = vmid
	f.record("snapshot-list")
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.snapshots, nil
}

func (f *fakeSnapshotClient) RollbackSnapshot(_ context.Context, _ *hostssh.RemoteISOParams, vmid int, name string, _ time.Duration) error {
	f.rollbackCalls++
	f.lastVMID = vmid
	f.lastRollbackName = name
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
	f.agentCalls++
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

func indicesOf(log []string, event string) []int {
	var out []int
	for i, e := range log {
		if e == event {
			out = append(out, i)
		}
	}
	return out
}

// TestCreateSnapshot_readyNodeCordonsDrainsAndUncordons is requirement (a):
// create on a Ready node does Cordon→Drain→CreateSnapshot→Uncordon in order.
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

// TestCreateSnapshot_notReadySkipsCordonAndDrain is requirement (b): create on
// a NotReady node is snapshotted directly with no cordon/drain/uncordon.
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

// TestCreateSnapshot_dryRunMakesZeroMutatingCalls is requirement (c) for create.
func TestCreateSnapshot_dryRunMakesZeroMutatingCalls(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	// seedRunner defaults DryRun to true.

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

// TestCreateSnapshot_defaultName covers the auto-generated name path.
func TestCreateSnapshot_defaultName(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)

	name, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if !strings.HasPrefix(name, "okdctl-") {
		t.Errorf("name = %q; want an okdctl- prefixed default", name)
	}
}

// TestCreateSnapshot_crashConsistencyWarningFires confirms the warning fires
// unconditionally when the agent is disabled — including under dry-run.
func TestCreateSnapshot_crashConsistencyWarningFires(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		t.Run(map[bool]string{true: "dry-run", false: "real"}[dryRun], func(t *testing.T) {
			fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
			fsc := &fakeSnapshotClient{agentEnabled: false}
			cfg := config.DefaultConfig()
			cfg.Provider.Proxmox.Node = testProxmoxNode

			r := seedSnapshotRunner(t, fc, fsc, cfg)
			r.DryRun = dryRun
			var buf bytes.Buffer
			r.Log = slog.New(slog.NewTextHandler(&buf, nil))

			if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err != nil {
				t.Fatalf("CreateSnapshot: %v", err)
			}
			if !strings.Contains(buf.String(), "crash-consistent only") {
				t.Errorf("warning did not fire; log:\n%s", buf.String())
			}
		})
	}
}

// TestCreateSnapshot_agentEnabledNoWarning ensures the warning is specific to
// the disabled/unprobeable case, not unconditional noise on every call.
func TestCreateSnapshot_agentEnabledNoWarning(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	fsc := &fakeSnapshotClient{agentEnabled: true}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	var buf bytes.Buffer
	r.Log = slog.New(slog.NewTextHandler(&buf, nil))

	if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if strings.Contains(buf.String(), "crash-consistent only") {
		t.Errorf("warning fired despite agent enabled; log:\n%s", buf.String())
	}
}

// TestCreateSnapshot_agentProbeFailureAssumesDisabled covers the best-effort
// probe: a probe error must not fail the op and must still warn.
func TestCreateSnapshot_agentProbeFailureAssumesDisabled(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	fsc := &fakeSnapshotClient{agentErr: errors.New("ssh timeout")}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	var buf bytes.Buffer
	r.Log = slog.New(slog.NewTextHandler(&buf, nil))

	if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err != nil {
		t.Fatalf("probe failure must not fail the op: %v", err)
	}
	if !strings.Contains(buf.String(), "crash-consistent only") {
		t.Errorf("probe failure must still warn crash-consistent-only; log:\n%s", buf.String())
	}
}

// TestCreateSnapshot_successClearsOpMarker is part of FIX 2: a clean snapshot
// leaves no op marker behind — cordonAndDrain's OpSnapshot marker must not
// outlive a successful op, so `okdctl node list` never shows a phantom
// in-flight op for a snapshot that already finished.
func TestCreateSnapshot_successClearsOpMarker(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	marker, err := ReadOpMarker(r.WorkDir, cfg.Cluster.Name)
	if err != nil {
		t.Fatalf("ReadOpMarker: %v", err)
	}
	if marker != nil {
		t.Errorf("expected no marker after a clean snapshot; got %+v", marker)
	}
}

// TestCreateSnapshot_failureLeavesOpSnapshotMarkerNotOpRemove is FIX 2: it
// proves the shared-marker hazard is closed. cordonAndDrain is shared between
// RemoveWorker and snapshot; before this fix it always wrote OpRemove, so a
// snapshot that cordoned/drained and then died left a marker `okdctl node
// remove` would resume-match. Here the pvesh create call itself fails after a
// successful cordon/drain, leaving a marker behind — it must be tagged
// OpSnapshot, never OpRemove, since any future resume logic keys off Op
// equality (see opstate.go's Op type).
func TestCreateSnapshot_failureLeavesOpSnapshotMarkerNotOpRemove(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	fsc := &fakeSnapshotClient{createErr: errors.New("no space left on device")}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err == nil {
		t.Fatal("expected error when the pvesh create call fails")
	}

	marker, err := ReadOpMarker(r.WorkDir, cfg.Cluster.Name)
	if err != nil {
		t.Fatalf("ReadOpMarker: %v", err)
	}
	if marker == nil {
		t.Fatal("expected a marker to remain after a cordoned failure")
	}
	if marker.Op != OpSnapshot {
		t.Errorf("marker.Op = %q; want %q", marker.Op, OpSnapshot)
	}
	if marker.Op == OpRemove {
		t.Fatal("marker.Op must never equal OpRemove: a later 'okdctl node remove' keys resume on Op equality")
	}
}

func TestCreateSnapshot_requiresProxmox(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	r, _, _ := seedRunner(t, fc, &fakeTF{}, config.DefaultConfig())

	if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{}); err == nil {
		t.Fatal("expected error when Proxmox is not configured")
	}
}

func TestListSnapshots(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker}}}
	fsc := &fakeSnapshotClient{snapshots: []hostssh.SnapshotInfo{{Name: testSnapshotName}}}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)

	got, err := r.ListSnapshots(context.Background(), "worker0")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(got) != 1 || got[0].Name != testSnapshotName {
		t.Errorf("got = %+v", got)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker}}}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	if err := r.DeleteSnapshot(context.Background(), "worker0", testSnapshotName); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	if fsc.deleteCalls != 1 || fsc.lastDeleteName != testSnapshotName {
		t.Errorf("deleteCalls=%d lastDeleteName=%q", fsc.deleteCalls, fsc.lastDeleteName)
	}
}

func TestDeleteSnapshot_dryRunMakesZeroMutatingCalls(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker}}}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)

	if err := r.DeleteSnapshot(context.Background(), "worker0", testSnapshotName); err != nil {
		t.Fatalf("dry-run DeleteSnapshot: %v", err)
	}
	if fsc.deleteCalls != 0 {
		t.Errorf("dry-run called DeleteSnapshot: deleteCalls=%d", fsc.deleteCalls)
	}
}

// TestRollbackSnapshot_workerHappyPath exercises a full non-master rollback:
// no etcd/ceph gates, cordon→rollback→ready-wait→uncordon.
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

	if err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName); err != nil {
		t.Fatalf("RollbackSnapshot: %v", err)
	}
	if fc.cordon != 1 || fc.uncordon != 1 || fsc.rollbackCalls != 1 {
		t.Errorf("cordon=%d uncordon=%d rollback=%d", fc.cordon, fc.uncordon, fsc.rollbackCalls)
	}
	if fc.etcdCalls != 0 || fc.cephCalls != 0 {
		t.Errorf("worker rollback must not gate on etcd/ceph: etcdCalls=%d cephCalls=%d", fc.etcdCalls, fc.cephCalls)
	}
}

// TestRollbackSnapshot_masterGatesHealthPrePostBeforeUncordon is requirement
// (d): rollback on a MASTER calls EtcdHealthy+CephHealthy pre AND post, in
// order, before the final Uncordon.
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

	if err := r.RollbackSnapshot(context.Background(), "master0", testSnapshotName); err != nil {
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

// TestRollbackSnapshot_postGateFailureLeavesNodeCordoned is requirement (e):
// a failed POST-rollback health gate returns an error and does NOT uncordon.
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

	err := r.RollbackSnapshot(context.Background(), "master0", testSnapshotName)
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

// TestRollbackSnapshot_taskFailureLeavesNodeCordoned covers the "ON ANY
// FAILURE FROM HERE" contract at the rollback task itself.
func TestRollbackSnapshot_taskFailureLeavesNodeCordoned(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}},
		etcdHealthy: true,
	}
	fsc := &fakeSnapshotClient{rollbackErr: errors.New("no space left on device")}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName)
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

// TestRollbackSnapshot_dryRunMakesZeroMutatingCalls is requirement (c) for
// rollback: a dry-run must never cordon/drain, roll back, or block on the
// pre-gate (mirroring compact's dry-run-never-blocks contract).
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

	if err := r.RollbackSnapshot(context.Background(), "master0", testSnapshotName); err != nil {
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

// TestRollbackSnapshot_successClearsOpMarker mirrors
// TestCreateSnapshot_successClearsOpMarker for rollback: FIX 2 requires the
// OpSnapshot marker cordonAndDrain wrote to be cleared on a fully clean run.
func TestRollbackSnapshot_successClearsOpMarker(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	if err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName); err != nil {
		t.Fatalf("RollbackSnapshot: %v", err)
	}

	marker, err := ReadOpMarker(r.WorkDir, cfg.Cluster.Name)
	if err != nil {
		t.Fatalf("ReadOpMarker: %v", err)
	}
	if marker != nil {
		t.Errorf("expected no marker after a clean rollback; got %+v", marker)
	}
}

// TestRollbackSnapshot_finalUncordonFailureSurfacesAsError is FIX 3: a failed
// final uncordon must surface as the op's result (matching CreateSnapshot's
// promote-to-error behavior) rather than demote to a warning behind a nil
// return — the command must never exit clean while the node is actually left
// cordoned. The OpSnapshot marker must also survive, since the op did not
// fully succeed.
func TestRollbackSnapshot_finalUncordonFailureSurfacesAsError(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}},
		uncordonErr: errors.New("connection refused"),
	}
	fsc := &fakeSnapshotClient{}
	cfg := config.DefaultConfig()
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r := seedSnapshotRunner(t, fc, fsc, cfg)
	r.DryRun = false

	err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName)
	if err == nil {
		t.Fatal("expected error when the final uncordon fails")
	}
	if !strings.Contains(err.Error(), "left cordoned") {
		t.Errorf("err = %q; want it to name the cordoned state", err.Error())
	}
	if fc.uncordon != 1 {
		t.Errorf("uncordon calls = %d; want 1 (attempted, even though it failed)", fc.uncordon)
	}

	marker, merr := ReadOpMarker(r.WorkDir, cfg.Cluster.Name)
	if merr != nil {
		t.Fatalf("ReadOpMarker: %v", merr)
	}
	if marker == nil || marker.Op != OpSnapshot {
		t.Errorf("marker = %+v; want a persisted OpSnapshot marker (not cleared on final uncordon failure)", marker)
	}
}

func TestRollbackSnapshot_requiresProxmox(t *testing.T) {
	fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
	r, _, _ := seedRunner(t, fc, &fakeTF{}, config.DefaultConfig())

	if err := r.RollbackSnapshot(context.Background(), "worker0", testSnapshotName); err == nil {
		t.Fatal("expected error when Proxmox is not configured")
	}
}
