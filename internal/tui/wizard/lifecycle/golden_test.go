package lifecycle

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type lifecycleScenario struct {
	name  string
	id    wizard.StepID
	build func() (*State, Hooks)
	seed  func(m *wizard.Model, st *State)
}

func removeWorkerPlan() *node.OpPlan {
	return &node.OpPlan{
		Op: node.OpRemove, Cluster: "homelab",
		Nodes: []node.PlanNode{{
			Name: "homelab-worker2", Role: nodetypes.RoleWorker,
			TFAddress: "m.worker[2]", Action: terraform.PlanActionDelete,
			OSDs: []string{"osd.1"}, Ingress: []string{"router-a"},
		}},
	}
}

func lifecycleScenarios() []lifecycleScenario {
	return []lifecycleScenario{
		{
			name: "op",
			id:   StepIDOp,
			build: func() (*State, Hooks) {
				return &State{Cfg: config.DefaultConfig()}, Hooks{}
			},
		},
		{
			name: "op_resume",
			id:   StepIDOp,
			build: func() (*State, Hooks) {
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "homelab"
				marker := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
				return &State{
					Cfg: cfg,
					Marker: &node.OpMarker{
						Op: node.OpResize, Target: "homelab-master0",
						Step: node.StepPowerCycle, Timestamp: marker,
					},
				}, Hooks{}
			},
			seed: func(m *wizard.Model, st *State) {
				marker := st.Marker.Timestamp
				if op, ok := m.CurrentStep().(*OpStep); ok {
					op.now = func() time.Time { return marker.Add(2 * time.Hour) }
				}
			},
		},
		{
			name: "target_resize",
			id:   StepIDTarget,
			build: func() (*State, Hooks) {
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "homelab"
				return &State{Cfg: cfg, Op: node.OpResize}, Hooks{}
			},
			seed: func(m *wizard.Model, _ *State) {
				m.Update(nodesLoadedMsg{nodes: []cluster.NodeDetail{
					{Name: "homelab-worker0", Role: nodetypes.RoleWorker, Ready: true},
					{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
				}})
				// Move selection into the dropdown so its header and a
				// highlighted node row render together.
				m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
				m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			},
		},
		{
			name: "target_remove",
			id:   StepIDTarget,
			build: func() (*State, Hooks) {
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "homelab"
				return &State{Cfg: cfg, Op: node.OpRemove}, Hooks{}
			},
			seed: func(m *wizard.Model, st *State) {
				// The jump through StepIDOp applies its default selection
				// (resize) first; restore remove before nodes load.
				st.Op = node.OpRemove
				m.Update(nodesLoadedMsg{nodes: []cluster.NodeDetail{
					{Name: "homelab-worker0", Role: nodetypes.RoleWorker, Ready: true},
					{Name: "homelab-worker2", Role: nodetypes.RoleWorker, Ready: true},
					{Name: "homelab-worker1", Role: nodetypes.RoleWorker, Ready: false},
					{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
				}})
			},
		},
		{
			name: "params",
			id:   StepIDParams,
			build: func() (*State, Hooks) {
				return &State{Cfg: config.DefaultConfig(), Op: node.OpResize}, Hooks{}
			},
		},
		{
			name: "preview",
			id:   StepIDPreview,
			build: func() (*State, Hooks) {
				return resizePreviewState(), Hooks{}
			},
			seed: func(m *wizard.Model, st *State) {
				st.Scope = node.ResizeScope{Role: nodetypes.RoleMaster}
				m.Update(dryRunDoneMsg{plan: masterResizePlan()})
			},
		},
		{
			name: "confirm",
			id:   StepIDConfirm,
			build: func() (*State, Hooks) {
				return &State{
					Cfg: config.DefaultConfig(), Op: node.OpRemove,
					Target: "homelab-worker2", Proceed: true,
					Plan: removeWorkerPlan(),
				}, Hooks{}
			},
		},
		{
			name: "exec",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return execState(), Hooks{}
			},
			seed: func(m *wizard.Model, _ *State) {
				m.Update(execEventMsg{ev: ExecEvent{Node: "homelab-master0", Step: node.StepTFApply}})
			},
		},
		{
			name: "done",
			id:   StepIDDone,
			build: func() (*State, Hooks) {
				st := doneState()
				st.Elapsed = 90 * time.Second
				return st, Hooks{}
			},
		},
	}
}

var lifecycleGoldenSizes = []struct {
	w, h int
	fits bool
}{
	{80, 24, true},
	{100, 30, true},
	{120, 40, true},
}

func lifecycleChrome() wizard.FlowChrome {
	return wizard.FlowChrome{
		Tagline: "okd over proxmox, the easy way",
		Badge:   func(c *config.Config) string { return c.Cluster.Name },
	}
}

func TestGolden_LifecycleSteps(t *testing.T) {
	for _, sz := range lifecycleGoldenSizes {
		for _, sc := range lifecycleScenarios() {
			t.Run(fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h), func(t *testing.T) {
				st, hooks := sc.build()
				m := wizard.NewFlowModel(NewSteps(st, hooks), st.Cfg, lifecycleChrome())

				_ = tuitest.RenderAt(t, m, sz.w, sz.h)
				m.Update(wizard.JumpToStepMsg{StepID: sc.id})
				if sc.seed != nil {
					sc.seed(m, st)
				}

				frame := tuitest.RenderAt(t, m, sz.w, sz.h)
				tuitest.Golden(t, fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h), frame)
				if sz.fits {
					tuitest.AssertFits(t, frame, sz.w, sz.h)
				}
			})
		}
	}
}
