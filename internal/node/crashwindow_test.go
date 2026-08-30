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

// These tests model a crash by seeding the on-disk op marker at the step an
// interrupted run was about to perform and mutating fake state to match, then
// proving a fresh Runner resumes instead of re-validating a stale baseline.

func TestRemoveWorkerResumesBetweenApplyAndPersist(t *testing.T) {
	const target = "worker2"

	// Crashed run: marker at tf-apply; the delete already landed in state, but
	// the process died before persisting worker_count or deleting the k8s Node.
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
	// Apply already landed and persist already ran, but the marker is still StepTFApply.
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
	cfg.Topology.Workers.Count = 2 // the tf-apply block's persist already ran

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
		// No pending change for master0 classifies it alreadyAtTarget (already landed before the crash).
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
	master0VMID := nodetypes.VMID(cfg, nodetypes.RoleMaster, 0)
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
	master1VMID := nodetypes.VMID(cfg, nodetypes.RoleMaster, 1)
	if !slices.Contains(fp.cycledVMIDs, master1VMID) {
		t.Errorf("master1 must be power-cycled on resume: vmids=%v want %d", fp.cycledVMIDs, master1VMID)
	}

	// master2: not yet reached — full sequence.
	if !slices.Contains(fc.cordonedNodes, "master2") || !slices.Contains(fc.drainedNodes, "master2") ||
		!slices.Contains(fc.uncordonedNodes, "master2") {
		t.Errorf("master2 must run the full cordon/drain/uncordon sequence: cordoned=%v drained=%v uncordoned=%v",
			fc.cordonedNodes, fc.drainedNodes, fc.uncordonedNodes)
	}
	master2VMID := nodetypes.VMID(cfg, nodetypes.RoleMaster, 2)
	if !slices.Contains(fp.cycledVMIDs, master2VMID) {
		t.Errorf("master2 must be power-cycled: vmids=%v want %d", fp.cycledVMIDs, master2VMID)
	}

	// master0 never reaches tf-apply and master1 resumes past it, so only master2 applies for real.
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
		fc := stopTestCluster()
		r, _, _ := seedRunner(t, fc, &fakeTF{}, config.DefaultConfig()) // DryRun defaults true
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

func TestResizeResumesMidPowerCycleWithMemberDown(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "master0", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "master1", Role: nodetypes.RoleMaster, Ready: true},
			{Name: "master2", Role: nodetypes.RoleMaster, Ready: true},
		},
		etcdHealthy: false, // marked master is down until the resumed power-cycle brings it back
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
	r.EtcdGateTimeout = 1 * time.Second // fails fast if the pre-gate wrongly runs against the down member
	seedMarker(t, r, OpResize, "master1", StepPowerCycle)

	if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576}); err != nil {
		t.Fatalf("resume with the marked master powered off must not deadlock on the pre-etcd gate: %v", err)
	}

	master1VMID := nodetypes.VMID(cfg, nodetypes.RoleMaster, 1)
	if !slices.Contains(fp.cycledVMIDs, master1VMID) {
		t.Errorf("master1 must be power-cycled on resume: vmids=%v want %d", fp.cycledVMIDs, master1VMID)
	}
	if !slices.Contains(fc.uncordonedNodes, "master1") {
		t.Errorf("master1 must return to service once healthy: uncordoned=%v", fc.uncordonedNodes)
	}
	// master2's own pre-gate passed after quorum was restored — the skip is scoped, not blanket.
	if !slices.Contains(fc.cordonedNodes, "master2") {
		t.Errorf("master2 must run fresh after the resume: cordoned=%v", fc.cordonedNodes)
	}
}

// TestResizeResumesAtDiskGrowAfterApplyLanded is the broader crash-window
// counterpart to TestResizeResumeFromStepDiskGrowRunsBothEtcdGates above: that
// test locks the etcd-gate fix for a single already-marked node, this one
// drives a full role roll. A 3-master disk-only resize crashes right after
// master0's terraform apply landed but its in-guest grow failed (marker parks
// at disk-grow, target master0). The resumed run must not re-apply master0's
// already-landed terraform change, must re-run its in-guest grow, and must
// still roll master1/master2 — never reached before the crash — through the
// full sequence.
func TestResizeResumesAtDiskGrowAfterApplyLanded(t *testing.T) {
	fc := &fakeCluster{
		nodes:       threeMasters(),
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	fg := &fakeDiskGrower{err: errors.New("debug pod evicted")}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.DiskGB = 50

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Disk = fg

	// First run: master0's terraform apply lands, then its in-guest grow
	// fails — the roll stops there with the marker parked at disk-grow.
	err := r.Resize(t.Context(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{OSDiskGB: 100})
	if err == nil {
		t.Fatal("first run should fail at grow")
	}
	m, merr := ReadOpMarker(r.workDir, r.Cfg.Cluster.Name)
	if merr != nil || m == nil || m.Step != StepDiskGrow || m.Target != testMasterNode {
		t.Fatalf("marker = %+v, %v; want step disk-grow on %s", m, merr, testMasterNode)
	}
	if ftf.applyCalls != 1 {
		t.Fatalf("first run applies = %d, want 1 (master0 only — the roll stops at the failed grow)", ftf.applyCalls)
	}

	// Second run resumes: the grower now succeeds. master0 must not
	// re-apply (already landed) but must re-grow; master1/master2, never
	// reached before the crash, must still roll in full.
	appliesBefore := ftf.applyCalls
	fg.err = nil
	if err := r.Resize(t.Context(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{OSDiskGB: 100}); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	masters := threeMasters()
	if got := ftf.applyCalls - appliesBefore; got != len(masters)-1 {
		t.Fatalf("applies on resume = %d, want %d (master1 + master2; master0's already-landed apply must not repeat)", got, len(masters)-1)
	}
	if len(fg.grown) != len(masters)+1 {
		t.Fatalf("grown = %v, want %d calls (master0 failed+retried, plus master1 and master2)", fg.grown, len(masters)+1)
	}
	if fg.grown[0] != testMasterNode || fg.grown[1] != testMasterNode {
		t.Fatalf("%s must be both the failed first attempt and the resumed retry: grown=%v", testMasterNode, fg.grown)
	}
	if !slices.Contains(fg.grown[1:], "master1") || !slices.Contains(fg.grown[1:], "master2") {
		t.Fatalf("master1 and master2 must both be grown on resume: grown=%v", fg.grown)
	}
	if fc.cordon != 0 || fc.drain != 0 || fc.uncordon != 0 {
		t.Errorf("disk-only resize must never cordon/drain/uncordon: cordon=%d drain=%d uncordon=%d", fc.cordon, fc.drain, fc.uncordon)
	}
}

// TestResizeResumeFromStepDiskGrowRunsBothEtcdGates covers the crash window
// around a disk-only resize's in-guest grow: the process died right after
// marking StepDiskGrow (mid GrowOSDisk, or just after it returned but before
// the marker advanced further). Unlike a resume at StepPowerCycle, the node
// here was never powered off — a disk-only resize never power-cycles at
// all — so neither the pre- nor the post-etcd-health gate has any deadlock
// risk to guard against, and both must run on resume. This locks the fix for
// a bug where the pre/post gates shared one "at or past power-cycle" flag:
// StepDiskGrow's slot in stepOrder sits at/after StepPowerCycle's, so that
// shared flag silently skipped BOTH gates on this exact resume.
func TestResizeResumeFromStepDiskGrowRunsBothEtcdGates(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	fg := &fakeDiskGrower{}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.DiskGB = 50

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Disk = fg
	seedMarker(t, r, OpResize, "master0", StepDiskGrow)

	// Role scope, not a single-node scope: --os-disk-gb is refused against a
	// single node (see TestResizeRefusesOSDiskGBOnSingleNode), so this test's
	// one-master fake cluster is targeted via its role, resolving to the same
	// sole node.
	if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{OSDiskGB: 100}); err != nil {
		t.Fatalf("resumed disk-grow resize: %v", err)
	}

	if len(fg.grown) != 1 {
		t.Fatalf("resumed run must re-run the in-guest grow: grown=%v", fg.grown)
	}
	if fc.etcdCalls != 2 {
		t.Fatalf("resumed disk-grow resume must run BOTH etcd gates (pre+post), the node was never powered off: etcdCalls=%d", fc.etcdCalls)
	}
}
