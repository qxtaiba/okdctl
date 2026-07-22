package node

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// These are C1's acceptance tests: they model a crash by seeding the on-disk
// op marker at the step an interrupted run was about to perform, mutating the
// fake cluster/terraform state to what the real world would look like after
// that partial run, then constructing a fresh Runner over that state — proving
// the SECOND call resumes (and completes) rather than re-running validation
// against a baseline the first run already moved past.

func TestRemoveWorkerResumesBetweenApplyAndPersist(t *testing.T) {
	const target = "worker2"

	// A fresh run, for comparison: it must run the full guard/ListNodes
	// sequence that the resumed run below skips.
	freshFC := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: target, Role: nodetypes.RoleWorker},
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
	}
	freshTF := &fakeTF{action: terraform.PlanActionDelete}
	freshCfg := config.DefaultConfig()
	freshCfg.Topology.Workers.Count = 3
	freshR, _, _ := seedRunner(t, freshFC, freshTF, freshCfg)
	freshR.DryRun = false

	if err := freshR.RemoveWorker(context.Background(), target, RemoveOptions{}); err != nil {
		t.Fatalf("fresh remove: %v", err)
	}
	if freshFC.listNodesCalls == 0 || freshFC.podsForSelectorCalls == 0 {
		t.Fatalf("fresh run baseline must call guards: listNodes=%d podsForSelector=%d",
			freshFC.listNodesCalls, freshFC.podsForSelectorCalls)
	}

	// The "crashed" run: a marker recorded at tf-apply (written just before
	// targetedApply is called) and never cleared. The real world it left
	// behind already applied the delete — state no longer has the address —
	// but the process died before decrementing worker_count/persisting and
	// before deleting the Kubernetes Node object.
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: target, Role: nodetypes.RoleWorker},
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete, emptyPlan: true, stateAbsent: true}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 3 // still the pre-crash value; persist never landed

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	seedMarker(t, r, OpRemove, target, StepTFApply)

	if err := r.RemoveWorker(context.Background(), target, RemoveOptions{}); err != nil {
		t.Fatalf("resumed remove: %v", err)
	}

	if fc.listNodesCalls != 0 || fc.podsForSelectorCalls != 0 {
		t.Errorf("resumed run must skip validateWorkerRemovable/removeGuards entirely: listNodes=%d podsForSelector=%d",
			fc.listNodesCalls, fc.podsForSelectorCalls)
	}
	if fc.listNodesCalls >= freshFC.listNodesCalls || fc.podsForSelectorCalls >= freshFC.podsForSelectorCalls {
		t.Errorf("resumed guard/ListNodes calls (%d/%d) must be fewer than a fresh run (%d/%d) — proves the skip is real, not coincidental",
			fc.listNodesCalls, fc.podsForSelectorCalls, freshFC.listNodesCalls, freshFC.podsForSelectorCalls)
	}
	if ftf.applyCalls != 0 {
		t.Errorf("already-applied delete must not be re-applied: applyCalls=%d", ftf.applyCalls)
	}
	if fc.deleteNode != 1 {
		t.Errorf("resumed remove must still delete the k8s Node object: deleteNode=%d", fc.deleteNode)
	}
	if cfg.Topology.Workers.Count != 2 {
		t.Errorf("resumed remove must persist the decremented worker_count: got %d, want 2", cfg.Topology.Workers.Count)
	}
}

func TestRemoveWorkerResumesBetweenApplyAndDeleteNode(t *testing.T) {
	const target = "worker2"

	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: target, Role: nodetypes.RoleWorker}, // still listed: the crash preceded DeleteNode
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	// The crash landed after the tf-apply block's persist already ran, so the
	// on-disk config already reflects the decremented count.
	cfg.Topology.Workers.Count = 2

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	seedMarker(t, r, OpRemove, target, StepDeleteK8s)

	if err := r.RemoveWorker(context.Background(), target, RemoveOptions{}); err != nil {
		t.Fatalf("resumed remove: %v", err)
	}

	if fc.cordon != 0 || fc.drain != 0 {
		t.Errorf("resumed remove must not re-cordon/drain: cordon=%d drain=%d", fc.cordon, fc.drain)
	}
	if ftf.planCalls != 0 || ftf.applyCalls != 0 {
		t.Errorf("resumed remove must not re-apply: planCalls=%d applyCalls=%d", ftf.planCalls, ftf.applyCalls)
	}
	if fc.deleteNode != 1 {
		t.Errorf("resumed remove must delete the k8s Node object: deleteNode=%d", fc.deleteNode)
	}
}

// TestResizeResumesMidMasterRoll drives a 3-master resize where the crashed
// run's marker names the middle master at power-cycle. The first master must
// have already landed (a read-only plan probe classifies it alreadyAtTarget
// and skips it with zero mutation); the marked master resumes at power-cycle
// with no re-cordon/drain/apply; the last master, never reached, runs the full
// sequence.
func TestResizeResumesMidMasterRoll(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "master0", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "master1", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "master2", Role: nodetypes.RoleMaster, Ready: true},
		},
		etcdHealthy: true,
	}
	ftf := &fakeTF{
		action: terraform.PlanActionUpdate,
		// master0's probe reports no pending change; StateHasResource's
		// default (present) makes planTargeted classify it alreadyAtTarget —
		// it already landed before the crash.
		emptyForAddress: map[string]bool{masterAddress(0): true},
	}
	fp := &fakePower{}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = fp
	seedMarker(t, r, OpResize, "master1", StepPowerCycle)

	if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576}); err != nil {
		t.Fatalf("resumed resize: %v", err)
	}

	// master0: already at target — the probe short-circuits with zero mutation.
	if slices.Contains(fc.cordonedNodes, "master0") || slices.Contains(fc.drainedNodes, "master0") ||
		slices.Contains(fc.uncordonedNodes, "master0") {
		t.Errorf("master0 (already at target) must not be touched: cordoned=%v drained=%v uncordoned=%v",
			fc.cordonedNodes, fc.drainedNodes, fc.uncordonedNodes)
	}
	master0VMID := cfg.Topology.VMIDBase + vmidMasterOffset
	if slices.Contains(fp.cycledVMIDs, master0VMID) {
		t.Errorf("master0 must not be power-cycled: vmids=%v", fp.cycledVMIDs)
	}

	// master1: resumes exactly at power-cycle.
	if slices.Contains(fc.cordonedNodes, "master1") || slices.Contains(fc.drainedNodes, "master1") {
		t.Errorf("master1 must not re-cordon/drain on resume: cordoned=%v drained=%v", fc.cordonedNodes, fc.drainedNodes)
	}
	if !slices.Contains(fc.uncordonedNodes, "master1") {
		t.Errorf("master1 must be uncordoned after resuming past power-cycle: uncordoned=%v", fc.uncordonedNodes)
	}
	master1VMID := cfg.Topology.VMIDBase + vmidMasterOffset + 1
	if !slices.Contains(fp.cycledVMIDs, master1VMID) {
		t.Errorf("master1 must be power-cycled on resume: vmids=%v want %d", fp.cycledVMIDs, master1VMID)
	}

	// master2: not yet reached — full sequence.
	if !slices.Contains(fc.cordonedNodes, "master2") || !slices.Contains(fc.drainedNodes, "master2") ||
		!slices.Contains(fc.uncordonedNodes, "master2") {
		t.Errorf("master2 must run the full cordon/drain/uncordon sequence: cordoned=%v drained=%v uncordoned=%v",
			fc.cordonedNodes, fc.drainedNodes, fc.uncordonedNodes)
	}
	master2VMID := cfg.Topology.VMIDBase + vmidMasterOffset + 2
	if !slices.Contains(fp.cycledVMIDs, master2VMID) {
		t.Errorf("master2 must be power-cycled: vmids=%v want %d", fp.cycledVMIDs, master2VMID)
	}

	// Exactly two power-cycles total (master1 resumed + master2 full) and
	// exactly one real apply (master2's — master0 never reaches tf-apply via
	// the probe short-circuit, master1 resumes past it).
	if fp.calls != 2 {
		t.Errorf("expected exactly 2 power-cycles (master1 + master2), got %d: vmids=%v", fp.calls, fp.cycledVMIDs)
	}
	if ftf.applyCalls != 1 {
		t.Errorf("expected exactly one real apply (master2 only), got %d", ftf.applyCalls)
	}
	if cfg.Topology.ControlPlane.MemoryMB != 24576 {
		t.Errorf("resize must persist the new sizing once the roll completes: got %d", cfg.Topology.ControlPlane.MemoryMB)
	}
}

func TestNodeCommandRefusesForeignMarkerWithoutAcknowledgment(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	fp := &fakePower{}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = fp
	seedMarker(t, r, OpRemove, "worker2", StepDrain)

	err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 16384, Acknowledge: false})
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError refusing the foreign marker, got %v", err)
	}
	for _, want := range []string{"worker2", "drain", "remove"} {
		if !strings.Contains(cfgErr.Error(), want) {
			t.Errorf("refusal must name the stranded op: %q does not contain %q", cfgErr.Error(), want)
		}
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 {
		t.Errorf("refused resize must make zero mutation: cordon=%d drain=%d uncordon=%d", fc.cordon, fc.drain, fc.uncordon)
	}
	if ftf.applyCalls != 0 || fp.calls != 0 {
		t.Errorf("refused resize must make zero mutation: applyCalls=%d powerCycles=%d", ftf.applyCalls, fp.calls)
	}

	if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 16384, Acknowledge: true}); err != nil {
		t.Fatalf("acknowledged resize must proceed fresh: %v", err)
	}
	if fc.cordon != 1 || fc.uncordon != 1 {
		t.Errorf("acknowledged resize should run the full sequence once: cordon=%d uncordon=%d", fc.cordon, fc.uncordon)
	}
	if cfg.Topology.ControlPlane.MemoryMB != 16384 {
		t.Errorf("acknowledged resize must persist the new sizing: got %d", cfg.Topology.ControlPlane.MemoryMB)
	}
}
