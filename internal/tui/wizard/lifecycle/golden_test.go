package lifecycle

import (
	"fmt"
	"testing"
	"time"

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
			name: "target",
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
	{80, 24, false},
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
