package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

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
		// OSD on target, force-allowed: storage guard warns but confirm still sees the OSD verdict.
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

func TestClusterPowerConfirmGate(t *testing.T) {
	tests := []struct {
		name      string
		op        Op
		approve   bool
		wantFirst nodetypes.NodeRole
	}{
		{"stop/approve", OpStop, true, nodetypes.RoleWorker},
		{"stop/decline", OpStop, false, nodetypes.RoleWorker},
		{"start/approve", OpStart, true, nodetypes.RoleMaster},
		{"start/decline", OpStart, false, nodetypes.RoleMaster},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := stopTestCluster()
			fp := &fakePower{}
			r, _, _ := seedRunner(t, fc, &fakeTF{}, startTestConfig())
			r.DryRun = false
			r.Power = fp
			r.ClusterReadyTimeout = 5 * time.Second

			var (
				fired      bool
				plan       OpPlan
				cordonAt   int
				shutdownAt int
				startAt    int
			)
			r.Confirm = func(_ context.Context, p *OpPlan) (bool, error) {
				fired = true
				plan = *p
				cordonAt, shutdownAt, startAt = fc.cordon, fp.shutdownCalls, fp.startCalls
				return tc.approve, nil
			}

			var err error
			switch tc.op {
			case OpStop:
				err = r.Stop(context.Background(), StopOptions{})
			case OpStart:
				err = r.Start(context.Background(), StartOptions{})
			}

			if !fired {
				t.Fatal("confirm hook never fired")
			}
			if cordonAt != 0 || shutdownAt != 0 || startAt != 0 {
				t.Errorf("mutation happened before confirm: cordon=%d shutdown=%d start=%d", cordonAt, shutdownAt, startAt)
			}
			if plan.Op != tc.op {
				t.Errorf("plan.Op = %q, want %q", plan.Op, tc.op)
			}
			if len(plan.Nodes) != 4 {
				t.Fatalf("plan should carry all 4 nodes; got %d", len(plan.Nodes))
			}
			if plan.Nodes[0].Role != tc.wantFirst {
				t.Errorf("plan ordering: first node role = %q, want %q", plan.Nodes[0].Role, tc.wantFirst)
			}

			if !tc.approve {
				if !errors.Is(err, ErrDeclined) {
					t.Fatalf("declined %s should return ErrDeclined: %v", tc.op, err)
				}
				if fc.cordon != 0 || fp.shutdownCalls != 0 || fp.startCalls != 0 {
					t.Errorf("declined %s mutated: cordon=%d shutdown=%d start=%d", tc.op, fc.cordon, fp.shutdownCalls, fp.startCalls)
				}
				return
			}

			if err != nil {
				t.Fatalf("approved %s: %v", tc.op, err)
			}
			switch tc.op {
			case OpStop:
				if fc.cordon != 4 || fp.shutdownCalls != 4 {
					t.Errorf("approved stop should run the full sequence: cordon=%d shutdown=%d", fc.cordon, fp.shutdownCalls)
				}
			case OpStart:
				if fp.startCalls != 4 {
					t.Errorf("approved start should power on every vm: start=%d", fp.startCalls)
				}
			}
		})
	}
}

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
	if fc.deleteNode != 2 {
		t.Errorf("both workers should be removed; deleteNode=%d", fc.deleteNode)
	}
	if fc.setSched != 1 || fc.applied != 1 {
		t.Errorf("control plane enabled once; setSched=%d applied=%d", fc.setSched, fc.applied)
	}
}
