package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

func TestDemoHooksListNodes(t *testing.T) {
	hooks := DemoHooks(0)
	nodes, err := hooks.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 6 {
		t.Fatalf("len(nodes) = %d, want 6", len(nodes))
	}
	var masters int
	for _, n := range nodes {
		if !n.Ready {
			t.Errorf("node %s must be ready in the demo fixture", n.Name)
		}
		if n.Role == nodetypes.RoleMaster {
			masters++
		}
	}
	if masters != 3 {
		t.Fatalf("masters = %d, want 3", masters)
	}
}

func TestDemoHooksDryRunPlanMatchesState(t *testing.T) {
	hooks := DemoHooks(0)
	st := &State{
		Cfg:    config.DefaultConfig(),
		Op:     node.OpRemove,
		Target: DemoClusterName + "-worker2",
	}
	plan, err := hooks.DryRun(st)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(plan.Nodes) != 1 {
		t.Fatalf("len(plan.Nodes) = %d, want 1", len(plan.Nodes))
	}
	if plan.Nodes[0].Name != st.Target || plan.Nodes[0].Action != terraform.PlanActionDelete {
		t.Fatalf("plan.Nodes[0] = %+v, want a delete node named %q", plan.Nodes[0], st.Target)
	}
}

// TestDemoHooksDryRunResizePlanMatchesState pins demoResizePlan, reachable
// only interactively (the resize op picked on the op screen, a role scoped
// on target) until now: every fixture master resolves into the plan, each
// as a terraform update, carrying st's sizing.
func TestDemoHooksDryRunResizePlanMatchesState(t *testing.T) {
	hooks := DemoHooks(0)
	st := &State{
		Cfg:      config.DefaultConfig(),
		Op:       node.OpResize,
		Scope:    node.ResizeScope{Role: nodetypes.RoleMaster},
		MemoryMB: 16384,
		CPU:      8,
	}
	plan, err := hooks.DryRun(st)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(plan.Nodes) != 3 {
		t.Fatalf("len(plan.Nodes) = %d, want 3 (the fixture's masters)", len(plan.Nodes))
	}
	for _, n := range plan.Nodes {
		if n.Role != nodetypes.RoleMaster || n.Action != terraform.PlanActionUpdate {
			t.Errorf("plan node %+v, want a master update", n)
		}
	}
	if plan.MemoryMB != st.MemoryMB || plan.CPU != st.CPU {
		t.Errorf("plan sizing = %d MiB / %d vCPU, want %d / %d", plan.MemoryMB, plan.CPU, st.MemoryMB, st.CPU)
	}
}

// TestDemoHooksDryRunAddPlanMatchesState pins demoAddPlan, reachable only
// interactively until now: the batch starts at the fixture's current
// worker count so a repeated add never collides with an existing name.
func TestDemoHooksDryRunAddPlanMatchesState(t *testing.T) {
	hooks := DemoHooks(0)
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpAdd, Count: 2}
	plan, err := hooks.DryRun(st)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(plan.Nodes) != st.Count {
		t.Fatalf("len(plan.Nodes) = %d, want %d", len(plan.Nodes), st.Count)
	}
	startIdx := st.Cfg.Topology.Workers.Count
	for i, n := range plan.Nodes {
		want := fmt.Sprintf("%s-worker%d", st.Cfg.Cluster.Name, startIdx+i)
		if n.Name != want || n.Role != nodetypes.RoleWorker || n.Action != terraform.PlanActionCreate {
			t.Errorf("plan.Nodes[%d] = %+v, want a create worker named %q", i, n, want)
		}
	}
}

func TestDemoHooksExecuteEmitsFinalAndHonoursCancel(t *testing.T) {
	demoRemoveState := func() *State {
		hooks := DemoHooks(0)
		nodes, err := hooks.ListNodes()
		if err != nil {
			t.Fatalf("ListNodes: %v", err)
		}
		st := &State{Cfg: config.DefaultConfig(), Op: node.OpRemove, Target: nodes[len(nodes)-1].Name}
		plan, err := hooks.DryRun(st)
		if err != nil {
			t.Fatalf("DryRun: %v", err)
		}
		st.Plan = plan
		return st
	}

	t.Run("completes with exactly one final event", func(t *testing.T) {
		hooks := DemoHooks(0)
		st := demoRemoveState()

		events := make(chan ExecEvent, 256)
		result := make(chan error, 1)
		go func() {
			err := hooks.Execute(st, events)
			events <- ExecEvent{Final: true, Err: err}
			close(events)
			result <- err
		}()

		var finals int
		for ev := range events {
			if ev.Final {
				finals++
			}
		}
		if finals != 1 {
			t.Fatalf("Final events = %d, want exactly 1", finals)
		}
		if err := <-result; err != nil {
			t.Fatalf("Execute returned %v, want nil", err)
		}
	})

	t.Run("cancel mid-way returns the ctx error", func(t *testing.T) {
		hooks := DemoHooks(0)
		st := demoRemoveState()

		// Unbuffered: Execute blocks on its next send until CancelOp fires,
		// so cancellation lands deterministically even with stepDelay 0.
		events := make(chan ExecEvent)
		result := make(chan error, 1)
		go func() { result <- hooks.Execute(st, events) }()

		<-events
		hooks.CancelOp()

		err := <-result
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute returned %v, want context.Canceled", err)
		}
	})
}
