package lifecycle

import (
	"errors"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
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
			// The skip-drain selection and its amber warning note, which
			// only render once the drain-mode field carries drainModeSkip.
			name: "params_drain_skip",
			id:   StepIDParams,
			build: func() (*State, Hooks) {
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "homelab"
				return &State{Cfg: cfg, Op: node.OpRemove, Target: "homelab-worker2"}, Hooks{}
			},
			seed: func(m *wizard.Model, st *State) {
				// The jump through StepIDOp applies its default selection
				// (resize) first; restore remove, then rebuild the form
				// for it (ParamsStep.Init lazily rebuilds and re-focuses
				// whenever the op changed). Remove's disruption section —
				// drain mode, timeout, force storage — has no sizing
				// section ahead of it, so drain mode is already focused.
				st.Op = node.OpRemove
				p, ok := m.CurrentStep().(*ParamsStep)
				if !ok {
					return
				}
				p.Init()
				m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
				// Scroll to the bottom so the narrow (80-col) golden shows
				// the amber warning View appends below the form, not just
				// the drain-mode selection.
				m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
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
			name: "preview_loading",
			id:   StepIDPreview,
			build: func() (*State, Hooks) {
				return resizePreviewState(), Hooks{}
			},
		},
		{
			name: "preview_error",
			id:   StepIDPreview,
			build: func() (*State, Hooks) {
				return resizePreviewState(), Hooks{}
			},
			seed: func(m *wizard.Model, _ *State) {
				m.Update(dryRunDoneMsg{err: errors.New("plan safety gate refused the change")})
			},
		},
		{
			name: "preview_blockers",
			id:   StepIDPreview,
			build: func() (*State, Hooks) {
				return &State{
					Cfg: config.DefaultConfig(), Op: node.OpRemove,
					Target: "homelab-worker2",
				}, Hooks{}
			},
			seed: func(m *wizard.Model, st *State) {
				// The jump through StepIDOp applies its default selection
				// (resize) first; restore remove before the dry-run seeds.
				st.Op = node.OpRemove
				st.Target = "homelab-worker2"
				m.Update(dryRunDoneMsg{plan: removeWorkerPlan()})
				// Scroll past the node table so the narrow (80-col) golden
				// shows the gate grid and the irreversible bar, not just
				// the section the "preview" scenario already covers.
				m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
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
			name: "confirm_partial",
			id:   StepIDConfirm,
			build: func() (*State, Hooks) {
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "homelab"
				return &State{
					Cfg: cfg, Op: node.OpRemove,
					Target: "homelab-worker2", Proceed: true,
					Plan: removeWorkerPlan(),
				}, Hooks{}
			},
			seed: func(m *wizard.Model, _ *State) {
				for _, r := range "homela" {
					m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
				}
			},
		},
		{
			name: "confirm_match",
			id:   StepIDConfirm,
			build: func() (*State, Hooks) {
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "homelab"
				return &State{
					Cfg: cfg, Op: node.OpRemove,
					Target: "homelab-worker2", Proceed: true,
					Plan: removeWorkerPlan(),
				}, Hooks{}
			},
			seed: func(m *wizard.Model, _ *State) {
				for _, r := range "homelab" {
					m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
				}
			},
		},
		{
			// Mid-run: m0 collapsed with its total, m1 expanded with a
			// spinner row and right-aligned durations, m2 still pending.
			name: "exec",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), Hooks{}
			},
			seed: func(m *wizard.Model, _ *State) {
				s, ok := m.CurrentStep().(*ExecStep)
				if !ok {
					return
				}
				base := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
				cur := base
				s.now = func() time.Time { return cur }
				s.started = base
				s.buildRows()

				s.applyEvent(&ExecEvent{Node: "homelab-master0", Step: node.StepTFApply})
				cur = base.Add(60 * time.Second)
				s.applyEvent(&ExecEvent{Node: "homelab-master1", Step: node.StepTFApply})
				cur = base.Add(75 * time.Second)
			},
		},
		{
			// The graceful-cancel amber line, shown under the headline
			// while the current gate finishes.
			name: "exec_cancel",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), Hooks{}
			},
			seed: func(m *wizard.Model, _ *State) {
				s, ok := m.CurrentStep().(*ExecStep)
				if !ok {
					return
				}
				base := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
				cur := base
				s.now = func() time.Time { return cur }
				s.started = base
				s.buildRows()

				s.applyEvent(&ExecEvent{Node: "homelab-master0", Step: node.StepTFApply})
				cur = base.Add(30 * time.Second)
				s.InterceptQuit()
			},
		},
		{
			// The finished state: every node has collapsed to a single
			// line with its own total duration.
			name: "exec_finished",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), Hooks{}
			},
			seed: func(m *wizard.Model, _ *State) {
				s, ok := m.CurrentStep().(*ExecStep)
				if !ok {
					return
				}
				base := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
				cur := base
				s.now = func() time.Time { return cur }
				s.started = base
				s.buildRows()

				s.applyEvent(&ExecEvent{Node: "homelab-master0", Step: node.StepTFApply})
				cur = base.Add(60 * time.Second)
				s.applyEvent(&ExecEvent{Node: "homelab-master1", Step: node.StepTFApply})
				cur = base.Add(140 * time.Second)
				s.applyEvent(&ExecEvent{Node: "homelab-master2", Step: node.StepTFApply})
				cur = base.Add(200 * time.Second)
				_, _ = s.Update(execEventMsg{ev: ExecEvent{Final: true}})
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
		{
			// The failure frame: render.ErrorCard inside the wizard chrome,
			// shown once st.Result carries the backend error.
			name: "done_failure",
			id:   StepIDDone,
			build: func() (*State, Hooks) {
				st := doneState()
				st.Elapsed = 90 * time.Second
				st.Result = errors.New("etcd health gate (post-master0) failed: quorum lost")
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

func TestGolden_LifecycleSteps(t *testing.T) {
	for _, sz := range lifecycleGoldenSizes {
		for _, sc := range lifecycleScenarios() {
			t.Run(fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h), func(t *testing.T) {
				// NewFlowModel seeds its initial size from the process's
				// real terminal (pipes under go test, or CI) rather than
				// sz; pin it so the golden is independent of that.
				tui.SetTerminalWidth(sz.w)
				t.Cleanup(func() { tui.SetTerminalWidth(0) })

				st, hooks := sc.build()
				m := wizard.NewFlowModel(NewSteps(st, hooks), st.Cfg, Chrome())

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
