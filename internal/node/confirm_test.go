package node

import (
	"context"
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// confirmSnapshot records the mutation-counter state observed at the instant the
// confirm hook fired, proving guards ran and no mutation happened before consent.
type confirmSnapshot struct {
	fired      bool
	plan       OpPlan
	cordon     int
	drain      int
	deleteNode int
	setSched   int
	applied    int
	tfApply    int
}

func TestRemoveConfirmRunsAfterGuardsBeforeMutation(t *testing.T) {
	fc := &fakeCluster{
		nodes: []cluster.NodeDetail{
			{Name: "worker0", Role: nodetypes.RoleWorker},
			{Name: "worker1", Role: nodetypes.RoleWorker},
			{Name: "master0", Role: nodetypes.RoleMaster},
		},
		schedulable: true,
		// OSD on the target, force-allowed: the storage guard runs, warns, and the
		// plan still reaches confirm carrying the OSD verdict — proof the read-only
		// guards executed before the prompt.
		osdPods: []cluster.PodPlacement{{Name: "osd-1", Namespace: "rook-ceph", NodeName: "worker1"}},
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 2

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = &fakePower{}

	var snap confirmSnapshot
	r.Confirm = func(_ context.Context, plan *OpPlan) (bool, error) {
		snap.fired = true
		snap.plan = *plan
		snap.cordon, snap.drain, snap.deleteNode = fc.cordon, fc.drain, fc.deleteNode
		snap.tfApply = ftf.applyCalls
		return false, nil // decline
	}

	if err := r.RemoveWorker(context.Background(), "worker1", RemoveOptions{ForceStorage: true, DrainTimeout: "10m"}); !errors.Is(err, ErrDeclined) {
		t.Fatalf("declined remove should return ErrDeclined: %v", err)
	}

	if !snap.fired {
		t.Fatal("confirm hook never fired")
	}
	if len(snap.plan.Nodes) != 1 || len(snap.plan.Nodes[0].OSDs) != 1 {
		t.Errorf("confirm did not observe the guard verdict (OSD placement); plan=%+v", snap.plan)
	}
	if snap.plan.Nodes[0].TFAddress != workerAddress(1) || snap.plan.Nodes[0].Action != terraform.PlanActionDelete {
		t.Errorf("plan node missing tf address/action: %+v", snap.plan.Nodes[0])
	}
	if snap.cordon != 0 || snap.drain != 0 || snap.deleteNode != 0 || snap.tfApply != 0 {
		t.Errorf("mutation happened before confirm: cordon=%d drain=%d delete=%d apply=%d",
			snap.cordon, snap.drain, snap.deleteNode, snap.tfApply)
	}
	// Declining leaves zero mutation.
	if fc.cordon != 0 || fc.drain != 0 || fc.deleteNode != 0 || ftf.applyCalls != 0 {
		t.Errorf("declined remove mutated: cordon=%d drain=%d delete=%d apply=%d",
			fc.cordon, fc.drain, fc.deleteNode, ftf.applyCalls)
	}
}

func TestCompactConfirmRunsAfterPreflightBeforeControlPlane(t *testing.T) {
	fc := &fakeCluster{
		nodes:       compactNodes(),
		schedulable: true,
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 2

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = &fakePower{}

	var snap confirmSnapshot
	r.Confirm = func(_ context.Context, plan *OpPlan) (bool, error) {
		snap.fired = true
		snap.plan = *plan
		snap.setSched, snap.applied = fc.setSched, fc.applied
		snap.deleteNode = fc.deleteNode
		snap.tfApply = ftf.applyCalls
		return false, nil
	}

	if err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2}); !errors.Is(err, ErrDeclined) {
		t.Fatalf("declined compact should return ErrDeclined: %v", err)
	}

	if !snap.fired {
		t.Fatal("confirm hook never fired")
	}
	// Preflight plan-gated every worker before confirm.
	if len(snap.plan.Nodes) != 2 {
		t.Errorf("compact plan should carry both workers; got %d", len(snap.plan.Nodes))
	}
	if snap.setSched != 0 || snap.applied != 0 || snap.deleteNode != 0 {
		t.Errorf("control plane mutated before confirm: setSched=%d applied=%d delete=%d",
			snap.setSched, snap.applied, snap.deleteNode)
	}
	if fc.setSched != 0 || fc.applied != 0 || fc.deleteNode != 0 {
		t.Errorf("declined compact mutated the control plane: setSched=%d applied=%d delete=%d",
			fc.setSched, fc.applied, fc.deleteNode)
	}
}

// TestResizeConfirmYesProceeds is the non-interactive analog: a confirm hook
// that approves (mirroring --yes) lets the resize run to completion.
func TestResizeConfirmYesProceeds(t *testing.T) {
	fc := &fakeCluster{
		nodes:       []cluster.NodeDetail{{Name: "master0", Role: nodetypes.RoleMaster, Ready: true}},
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionUpdate}
	cfg := config.DefaultConfig()
	cfg.Topology.ControlPlane.MemoryMB = 12288

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = &fakePower{}

	var planMemory int
	r.Confirm = func(_ context.Context, plan *OpPlan) (bool, error) {
		planMemory = plan.MemoryMB
		return true, nil
	}

	if err := r.Resize(context.Background(), ResizeScope{Role: nodetypes.RoleMaster}, ResizeOptions{MemoryMB: 24576}); err != nil {
		t.Fatalf("approved resize: %v", err)
	}
	if planMemory != 24576 {
		t.Errorf("confirm plan memory = %d, want 24576", planMemory)
	}
	if ftf.applyCalls != 1 {
		t.Errorf("approved resize should apply once; applyCalls=%d", ftf.applyCalls)
	}
}

// TestCompactConsentFiresExactlyOnceThroughLoop drives an approving compact all
// the way through its remove-every-worker loop and asserts the consent gate
// fired EXACTLY ONCE for the whole compact — not once per inner RemoveWorker.
// The plan the hook last saw (what the CLI records as rc.captured for the
// completion box) must be the compact plan, never an inner remove plan.
func TestCompactConsentFiresExactlyOnceThroughLoop(t *testing.T) {
	fc := &fakeCluster{
		nodes:       compactNodes(),
		schedulable: true,
		etcdHealthy: true,
	}
	ftf := &fakeTF{action: terraform.PlanActionDelete}
	cfg := config.DefaultConfig()
	cfg.Topology.Workers.Count = 2

	r, _, _ := seedRunner(t, fc, ftf, cfg)
	r.DryRun = false
	r.Power = &fakePower{}

	var fires int
	var lastPlan OpPlan
	r.Confirm = func(_ context.Context, plan *OpPlan) (bool, error) {
		fires++
		lastPlan = *plan
		return true, nil
	}

	if err := r.Compact(context.Background(), CompactOptions{IngressReplicas: 2}); err != nil {
		t.Fatalf("approved compact should complete: %v", err)
	}

	if fires != 1 {
		t.Errorf("consent gate fired %d time(s); want exactly 1 for the whole compact", fires)
	}
	if lastPlan.Op != OpCompact {
		t.Errorf("completion box would render %q plan, want the compact plan", lastPlan.Op)
	}
	if len(lastPlan.Nodes) != 2 {
		t.Errorf("compact plan should carry both workers; got %d", len(lastPlan.Nodes))
	}
	// The loop ran to completion under the single grant: both workers removed,
	// the control plane made schedulable once with the compact ingress applied.
	if fc.deleteNode != 2 {
		t.Errorf("both workers should be removed; deleteNode=%d", fc.deleteNode)
	}
	if fc.setSched != 1 || fc.applied != 1 {
		t.Errorf("control plane enabled once; setSched=%d applied=%d", fc.setSched, fc.applied)
	}
}
