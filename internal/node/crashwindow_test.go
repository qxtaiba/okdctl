package node

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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

// TestRemoveWorkerResumesBetweenPersistAndDeleteMarker covers the exact window
// the other crash-window tests miss: the crash lands AFTER persistTopology (the
// on-disk config already reads count=N-1) but BEFORE the delete-node marker
// advances, so the marker is still StepTFApply. A relative decrement would
// re-apply count-1 against the already-decremented config and land at N-2 —
// understating the topology so the next deploy destroys a healthy worker. The
// absolute (Count = idx) assignment must leave the config at N-1 on resume.
func TestRemoveWorkerResumesBetweenPersistAndDeleteMarker(t *testing.T) {
	const target = "worker2"

	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: target, Role: nodetypes.RoleWorker}, // still listed: DeleteNode never ran
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
	}
	// The apply already landed (state no longer holds worker[2], plan is empty),
	// and persist already ran — so the config reads the decremented count while
	// the marker is still StepTFApply.
	ftf := &fakeTF{action: terraform.PlanActionDelete, emptyPlan: true, stateAbsent: true}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 2 // persist LANDED before the crash

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	seedMarker(t, r, OpRemove, target, StepTFApply)

	if err := r.RemoveWorker(context.Background(), target, RemoveOptions{}); err != nil {
		t.Fatalf("resumed remove: %v", err)
	}

	if cfg.Topology.Workers.Count != 2 {
		t.Errorf("resumed remove must leave count at N-1 (2), not decrement again to N-2: got %d", cfg.Topology.Workers.Count)
	}
	if ftf.applyCalls != 0 {
		t.Errorf("already-applied delete must not be re-applied: applyCalls=%d", ftf.applyCalls)
	}
	if fc.deleteNode != 1 {
		t.Errorf("resumed remove must delete the k8s Node object: deleteNode=%d", fc.deleteNode)
	}
	var remaining []string
	for _, n := range fc.nodes {
		if n.Role == nodetypes.RoleWorker {
			remaining = append(remaining, n.Name)
		}
	}
	if !slices.Equal(remaining, []string{"worker0", "worker1"}) {
		t.Errorf("the correct workers must remain after resume: got %v, want [worker0 worker1]", remaining)
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

// TestCompactRefusesGenuinelyForeignMarkerBeforeControlPlaneMutation locks the
// Fix 2 top-check: a stranded marker for an op compact does NOT compose (here a
// stop-family marker) must be refused before enableSchedulableAndIngress runs,
// so zero control-plane mutation happens. An OpRemove/OpResize marker is NOT
// refused here (it is left to the inner beginOp resume) — that path is covered
// by the resume tests above.
func TestCompactRefusesGenuinelyForeignMarkerBeforeControlPlaneMutation(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 1

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	seedMarker(t, r, Op("stop"), "master0", StepDrain)

	err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2, Acknowledge: false})
	var cfgErr *errtypes.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want *errtypes.ConfigError refusing the foreign marker, got %v", err)
	}
	if !strings.Contains(cfgErr.Error(), "stop") {
		t.Errorf("refusal must name the stranded op: %q", cfgErr.Error())
	}
	if fc.setSched != 0 || fc.applied != 0 {
		t.Errorf("foreign-marker refusal must not touch the control plane: setSched=%d applied=%d", fc.setSched, fc.applied)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 || ftf.applyCalls != 0 {
		t.Errorf("foreign-marker refusal must make zero mutation: cordon=%d drain=%d delete=%d apply=%d",
			fc.cordon, fc.drain, fc.deleteNode, ftf.applyCalls)
	}
}

// TestDryRunPreviewsPastForeignMarker locks Fix 4: a --dry-run against a
// stranded foreign marker must preview the fresh plan, not refuse — the run
// mutates nothing, so resume is irrelevant. Covers remove, resize, and compact
// (compact's genuinely-foreign refusal is also gated off under dry-run).
func TestDryRunPreviewsPastForeignMarker(t *testing.T) {
	t.Run("remove", func(t *testing.T) {
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
		r, _, _ := seedRunner(t, fc, ftf, cfg) // DryRun defaults true
		seedMarker(t, r, OpResize, "master0", StepPowerCycle)

		if err := r.RemoveWorker(context.Background(), "worker2", RemoveOptions{}); err != nil {
			t.Fatalf("dry-run remove past foreign marker must preview, not refuse: %v", err)
		}
		if fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 || ftf.applyCalls != 0 {
			t.Errorf("dry-run must make zero mutation: cordon=%d drain=%d delete=%d apply=%d",
				fc.cordon, fc.drain, fc.deleteNode, ftf.applyCalls)
		}
	})

	t.Run("compact", func(t *testing.T) {
		fc := &fakeCluster{
			nodes: []cluster.NodeDetail{
				{Name: "worker0", Role: nodetypes.RoleWorker},
				{Name: "master0", Role: nodetypes.RoleMaster},
			},
			schedulable: true, etcdHealthy: true,
		}
		ftf := &fakeTF{action: terraform.PlanActionDelete}
		cfg := config.DefaultConfig()
		cfg.Topology.Workers.Count = 1
		r, _, _ := seedRunner(t, fc, ftf, cfg)             // DryRun defaults true
		seedMarker(t, r, Op("stop"), "master0", StepDrain) // genuinely foreign, yet previews under dry-run

		if err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2}); err != nil {
			t.Fatalf("dry-run compact past foreign marker must preview, not refuse: %v", err)
		}
		if fc.setSched != 0 || fc.applied != 0 || fc.deleteNode != 0 || ftf.applyCalls != 0 {
			t.Errorf("dry-run compact must make zero mutation: setSched=%d applied=%d delete=%d apply=%d",
				fc.setSched, fc.applied, fc.deleteNode, ftf.applyCalls)
		}
	})

	t.Run("resize", func(t *testing.T) {
		fc := &fakeCluster{
			nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
			etcdHealthy: true,
		}
		ftf := &fakeTF{action: terraform.PlanActionUpdate}
		cfg := config.DefaultConfig()
		cfg.Topology.ControlPlane.MemoryMB = 12288
		r, _, _ := seedRunner(t, fc, ftf, cfg)
		seedMarker(t, r, OpRemove, "worker2", StepDrain)
		if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576}); err != nil {
			t.Fatalf("dry-run resize past foreign marker must preview, not refuse: %v", err)
		}
		if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 || ftf.applyCalls != 0 {
			t.Errorf("dry-run must make zero mutation: cordon=%d drain=%d uncordon=%d apply=%d",
				fc.cordon, fc.drain, fc.uncordon, ftf.applyCalls)
		}
	})

	t.Run("snapshot create", func(t *testing.T) {
		fc := &fakeCluster{nodes: []cluster.NodeDetail{{Name: "worker0", Role: nodetypes.RoleWorker, Ready: true}}}
		fsc := &fakeSnapshotClient{}
		cfg := config.DefaultConfig()
		cfg.Provider.Proxmox.Node = testProxmoxNode
		r := seedSnapshotRunner(t, fc, fsc, cfg) // DryRun defaults true
		seedMarker(t, r, OpRemove, "worker5", StepDrain)

		if _, err := r.CreateSnapshot(context.Background(), "worker0", SnapshotCreateOptions{Name: testSnapshotName}); err != nil {
			t.Fatalf("dry-run snapshot create past foreign marker must preview, not refuse: %v", err)
		}
		if fc.cordon != 0 || fc.drain != 0 || fsc.createCalls != 0 {
			t.Errorf("dry-run must make zero mutation: cordon=%d drain=%d create=%d", fc.cordon, fc.drain, fsc.createCalls)
		}
	})

	t.Run("cluster stop", func(t *testing.T) {
		fc := &fakeCluster{nodes: stopTestNodes(), signerNotAfter: time.Now().Add(60 * 24 * time.Hour)}
		ftf := &fakeTF{}
		r, _, _ := seedRunner(t, fc, ftf, config.DefaultConfig()) // DryRun defaults true
		seedMarker(t, r, OpRemove, "worker5", StepDrain)

		if err := r.Stop(context.Background(), StopOptions{}); err != nil {
			t.Fatalf("dry-run stop past foreign marker must preview, not refuse: %v", err)
		}
		if fc.cordon != 0 {
			t.Errorf("dry-run must make zero mutation: cordon=%d", fc.cordon)
		}
	})

	t.Run("cluster start", func(t *testing.T) {
		fc := &fakeCluster{nodes: startTestNodes()}
		r, _, _ := seedRunner(t, fc, &fakeTF{}, startTestConfig()) // DryRun defaults true
		seedMarker(t, r, OpRemove, "worker5", StepDrain)

		if err := r.Start(context.Background(), StartOptions{}); err != nil {
			t.Fatalf("dry-run start past foreign marker must preview, not refuse: %v", err)
		}
		if fc.listNodesCalls != 0 {
			t.Errorf("dry-run must make zero mutation: listNodes=%d", fc.listNodesCalls)
		}
	})
}

// TestResizeResumesMidPowerCycleWithMemberDown covers the crash window inside
// PowerCycleVM itself: the stop landed but the start did not, so the marked
// master sits powered off and etcd cannot report healthy until it is powered
// back on. The resumed run must skip the pre-node etcd gate for that master —
// running it would deadlock behind the very power-on it blocks — while the
// post-cycle gate still verifies quorum before the node returns to service.
func TestResizeResumesMidPowerCycleWithMemberDown(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "master0", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "master1", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "master2", Role: nodetypes.RoleMaster, Ready: true},
		},
		// The marked master is down: etcd is degraded until the resumed
		// power-cycle brings it back.
		etcdHealthy: false,
	}
	ftf := &fakeTF{
		action:          terraform.PlanActionUpdate,
		emptyForAddress: map[string]bool{masterAddress(0): true},
	}
	fp := &fakePower{}
	fp.onCycle = func() { fc.etcdHealthy = true }
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288
	cfg.Topology.VMIDBase = 6000
	cfg.Provider.Proxmox.Node = testProxmoxNode

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = fp
	// Keep the failure mode fast: if the pre-gate wrongly runs against the
	// down member, it times out here instead of burning the default gate.
	r.EtcdGateTimeout = 1 * time.Second
	seedMarker(t, r, OpResize, "master1", StepPowerCycle)

	if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576}); err != nil {
		t.Fatalf("resume with the marked master powered off must not deadlock on the pre-etcd gate: %v", err)
	}

	master1VMID := cfg.Topology.VMIDBase + vmidMasterOffset + 1
	if !slices.Contains(fp.cycledVMIDs, master1VMID) {
		t.Errorf("master1 must be power-cycled on resume: vmids=%v want %d", fp.cycledVMIDs, master1VMID)
	}
	if !slices.Contains(fc.uncordonedNodes, "master1") {
		t.Errorf("master1 must return to service once healthy: uncordoned=%v", fc.uncordonedNodes)
	}
	// master2 ran fresh AFTER the cycle restored quorum, so its own pre-gate
	// passed — proving the skip is scoped to the resumed node, not a blanket
	// gate removal.
	if !slices.Contains(fc.cordonedNodes, "master2") {
		t.Errorf("master2 must run fresh after the resume: cordoned=%v", fc.cordonedNodes)
	}
}
