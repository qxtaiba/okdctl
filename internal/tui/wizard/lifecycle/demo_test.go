package lifecycle

import (
	"context"
	"errors"
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
