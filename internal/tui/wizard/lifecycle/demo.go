package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
)

// DemoClusterName is the fixed cluster identity DemoHooks' static node list
// carries, so a CLI-built demo config that shares this name resolves every
// node the operator picks against the same fixture.
const DemoClusterName = "homelab"

// demoTFPrefix mirrors internal/node's unexported terraform-address prefix
// for OKD VM resources, so the demo preview's addresses read like the real ones.
const demoTFPrefix = "module.okd_cluster.proxmox_virtual_environment_vm."

// DemoHooks returns Hooks that drive the lifecycle wizard against a static
// six-node fixture and a GateRows-derived execution script paced by
// stepDelay, so every screen renders without a live Proxmox/OKD cluster.
func DemoHooks(stepDelay time.Duration) Hooks {
	// ctx has no caller context to inherit — a demo run's only cancellation
	// source is the wizard's exec-step graceful cancel, wired through CancelOp.
	ctx, cancel := context.WithCancel(context.Background())
	return Hooks{
		ListNodes: func() ([]cluster.NodeDetail, error) { return demoNodes(), nil },
		DryRun:    demoPlan,
		CancelOp:  cancel,
		Execute: func(st *State, events chan<- ExecEvent) error {
			return demoExecute(ctx, st, events, stepDelay)
		},
	}
}

// demoNodes returns the static fixture ListNodes serves: three masters and
// three workers, all ready, named after DemoClusterName.
func demoNodes() []cluster.NodeDetail {
	nodes := make([]cluster.NodeDetail, 0, 6)
	for i := range 3 {
		nodes = append(nodes, cluster.NodeDetail{
			Name: fmt.Sprintf("%s-master%d", DemoClusterName, i), Role: nodetypes.RoleMaster, Ready: true,
		})
	}
	for i := range 3 {
		nodes = append(nodes, cluster.NodeDetail{
			Name: fmt.Sprintf("%s-worker%d", DemoClusterName, i), Role: nodetypes.RoleWorker, Ready: true,
		})
	}
	return nodes
}

// demoPlan builds the OpPlan DryRun hands the preview screen, derived from
// st the same way the real DryRun hooks derive one from the live cluster.
func demoPlan(st *State) (*node.OpPlan, error) {
	switch st.Op {
	case node.OpRemove:
		return demoRemovePlan(st)
	case node.OpResize:
		return demoResizePlan(st)
	case node.OpAdd:
		return demoAddPlan(st), nil
	default:
		return nil, fmt.Errorf("build demo plan: unsupported op %q", st.Op)
	}
}

func demoRemovePlan(st *State) (*node.OpPlan, error) {
	idx, ok := cluster.NodeIndex(st.Target)
	if !ok {
		return nil, fmt.Errorf("build demo plan: derive terraform index from node name %q", st.Target)
	}
	return &node.OpPlan{
		Op:           node.OpRemove,
		Cluster:      st.Cfg.Cluster.Name,
		DrainTimeout: st.DrainTimeout,
		Nodes: []node.PlanNode{{
			Name:      st.Target,
			Role:      nodetypes.RoleWorker,
			TFAddress: demoAddress(nodetypes.RoleWorker, idx),
			Action:    terraform.PlanActionDelete,
		}},
	}, nil
}

// demoResizePlan resolves st.Scope against st.Nodes (the fixture TargetStep
// already loaded) so the plan matches whatever the operator actually picked.
func demoResizePlan(st *State) (*node.OpPlan, error) {
	nodes := st.Nodes
	if len(nodes) == 0 {
		nodes = demoNodes()
	}

	var targets []cluster.NodeDetail
	if st.Scope.Node != "" {
		for _, n := range nodes {
			if n.Name == st.Scope.Node {
				targets = []cluster.NodeDetail{n}
				break
			}
		}
	} else {
		for _, n := range nodes {
			if n.Role == st.Scope.Role {
				targets = append(targets, n)
			}
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("build demo plan: no nodes match resize scope %+v", st.Scope)
	}

	planNodes := make([]node.PlanNode, 0, len(targets))
	for _, n := range targets {
		idx, ok := cluster.NodeIndex(n.Name)
		if !ok {
			continue
		}
		planNodes = append(planNodes, node.PlanNode{
			Name:      n.Name,
			Role:      n.Role,
			TFAddress: demoAddress(n.Role, idx),
			Action:    terraform.PlanActionUpdate,
		})
	}
	return &node.OpPlan{
		Op: node.OpResize, Cluster: st.Cfg.Cluster.Name, Nodes: planNodes,
		MemoryMB: st.MemoryMB, CPU: st.CPU, OSDiskGB: st.OSDiskGB,
	}, nil
}

// demoAddPlan starts the new batch at the fixture's current worker count, so
// a repeated add in the same demo session never collides with an existing name.
func demoAddPlan(st *State) *node.OpPlan {
	count := max(st.Count, 1)
	startIdx := st.Cfg.Topology.Workers.Count
	nodes := make([]node.PlanNode, count)
	for i := range nodes {
		idx := startIdx + i
		nodes[i] = node.PlanNode{
			Name:      fmt.Sprintf("%s-worker%d", st.Cfg.Cluster.Name, idx),
			Role:      nodetypes.RoleWorker,
			TFAddress: demoAddress(nodetypes.RoleWorker, idx),
			Action:    terraform.PlanActionCreate,
		}
	}
	return &node.OpPlan{Op: node.OpAdd, Cluster: st.Cfg.Cluster.Name, Nodes: nodes}
}

func demoAddress(role nodetypes.NodeRole, idx int) string {
	kind := "worker"
	if role == nodetypes.RoleMaster {
		kind = "master"
	}
	return fmt.Sprintf("%s%s[%d]", demoTFPrefix, kind, idx)
}

// demoExecute replays a scripted event feed for every plan node, honouring
// ctx cancellation (wired to Hooks.CancelOp) between every event so a
// graceful cancel on the exec screen unblocks promptly instead of running
// the fixture to completion.
func demoExecute(ctx context.Context, st *State, events chan<- ExecEvent, stepDelay time.Duration) error {
	if st.Plan == nil {
		return nil
	}
	for i := range st.Plan.Nodes {
		for _, ev := range demoStepEvents(st, &st.Plan.Nodes[i]) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case events <- ev:
			}
			if err := demoWait(ctx, stepDelay); err != nil {
				return err
			}
		}
	}
	return nil
}

// demoWait pauses for d, or returns ctx's error the moment it's cancelled —
// a timer instead of time.Sleep so cancellation is never left waiting out a step.
func demoWait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// demoStepEvents scripts node's gate rows in order (GateRows is the single
// source both this and the exec screen's checklist read from), so the
// fixture drives exactly the rows the operator was shown on the preview screen.
func demoStepEvents(st *State, n *node.PlanNode) []ExecEvent {
	role := n.Role
	if role == "" {
		role = nodetypes.RoleWorker
	}
	var evs []ExecEvent
	for _, row := range GateRows(st.Op, role, st.SkipDrain, diskModeFor(st)) {
		evs = append(evs, demoRowScript(row, n.Name, n.TFAddress)...)
	}
	return evs
}

// demoRowScript returns the OnStep/Reporter events that drive one gate row
// to completion, matched back onto the row by the same prefixes match.go's
// descRowHints and stepRowHints resolve events onto rows with.
func demoRowScript(row, name, tfAddress string) []ExecEvent {
	const wait = 2 * time.Second
	switch {
	case row == rowCordonDrain:
		return demoSpan(name, "cordoning and draining "+name, time.Second, node.StepCordon, node.StepDrain)
	case strings.Contains(row, "terraform apply"):
		return demoSpan(name, "applying terraform change to "+tfAddress, 3*time.Second, node.StepTFApply)
	case strings.Contains(row, "power-cycle"):
		return demoSpan(name, "power-cycling vm to realize the new sizing", wait, node.StepPowerCycle)
	case strings.Contains(row, "wait for node ready"):
		return demoSpan(name, "waiting for node "+name+" to become ready", wait)
	case strings.Contains(row, "grow os disk"):
		return demoSpan(name, "growing os disk on "+name, wait, node.StepDiskGrow)
	case strings.Contains(row, "etcd health gate (pre)"):
		return demoSpan(name, "waiting for etcd health (pre-"+name+")", wait)
	case strings.Contains(row, "etcd health gate (post)"):
		return demoSpan(name, "waiting for etcd health (post-"+name+")", wait)
	case strings.Contains(row, "delete kubernetes node"):
		return []ExecEvent{{Node: name, Step: node.StepDeleteK8s}}
	case strings.Contains(row, "ceph health gate"):
		var evs []ExecEvent
		if strings.Contains(row, "uncordon") {
			evs = append(evs, ExecEvent{Node: name, Step: node.StepUncordon})
		}
		return append(evs, demoSpan(name, "waiting for ceph health ("+name+")", wait)...)
	case row == rowBuildISO:
		return []ExecEvent{{Node: name, Step: node.StepBuildISO}}
	case row == rowUploadISO:
		return []ExecEvent{{Node: name, Step: node.StepUploadISO}}
	case strings.Contains(row, "wait for join"):
		return []ExecEvent{{Node: name, Step: node.StepWaitJoin}}
	default:
		return nil
	}
}

// demoSpan scripts a Reporter-style span: a start Desc, an OnStep transition
// for each of steps (in order), then the closing Desc+Done+Took pair.
func demoSpan(name, desc string, took time.Duration, steps ...node.Step) []ExecEvent {
	evs := make([]ExecEvent, 0, len(steps)+2)
	evs = append(evs, ExecEvent{Node: name, Desc: desc})
	for _, step := range steps {
		evs = append(evs, ExecEvent{Node: name, Step: step})
	}
	return append(evs, ExecEvent{Node: name, Desc: desc, Done: true, Took: took})
}
