package lifecycle

import (
	"errors"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type lifecycleScenario struct {
	name  string
	id    wizard.StepID
	build func() (*State, Hooks)
	seed  func(m *wizard.Model, st *State)
}

// goldenHooks is the seeded feed the exec and done goldens render against:
// no backend, a fixed log ring, and a fixed sink path so the pane, the
// narrow tail, and the full-log pointers are deterministic.
func goldenHooks() Hooks {
	return Hooks{Logs: seededRing(24), LogPath: "okd-install/okdctl.log"}
}

// errorEarlyRing seeds a 24-line ring with one ERROR early on, so the jump
// keys and the minimap lane both have an off-screen target — exec's
// equivalent of deployexec's errorEarlyRing.
func errorEarlyRing() Hooks {
	r := logview.NewRing(logview.DefaultCap)
	for i := range 24 {
		line := logview.Line{
			At:    logBase.Add(time.Duration(i*7) * time.Second),
			Level: "INFO",
			Text:  fmt.Sprintf("terraform apply step=step-%02d node=homelab-master0", i),
		}
		if i == 4 {
			line.Level = "ERROR"
			line.Text = "etcd member 0 refused the join request"
		}
		r.Append(line)
	}
	return Hooks{Logs: r, LogPath: "okd-install/okdctl.log"}
}

// typeFilter opens the filter input on the active step and types pattern
// into it, one keystroke at a time, the way an operator would.
func typeFilter(m *wizard.Model, pattern string) {
	m.Update(tea.KeyPressMsg{Code: logview.KeyFilter, Text: "/"})
	for _, r := range pattern {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// seedExecMidRun drives the exec step to a fixed mid-run frame: m0
// collapsed with its total, m1 expanded around a running row, m2 pending.
func seedExecMidRun(m *wizard.Model, _ *State) {
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
}

// seedExecFailure drives the operation into master1's etcd gate and fails it
// there, then advances to the done screen — the path that hands the incident
// report its frozen checklist, which a State built by hand cannot produce.
func seedExecFailure(m *wizard.Model, _ *State) {
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
	cur = base.Add(90 * time.Second)
	s.Update(execEventMsg{ev: ExecEvent{Final: true, Err: errGoldenFailure}})
	m.Update(wizard.JumpToStepMsg{StepID: StepIDDone})
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
				// Six nodes: the reported case where a tall terminal must
				// grow the dropdown window past its old 5-row cap instead
				// of showing "1 more" above acres of empty space. At 80x24
				// the masters/workers rows above the dropdown leave less
				// room than even the floor of 5 needs, so the box's
				// closing border scrolls out of the 24-row viewport and
				// the generic "scroll down for more" footer takes over —
				// expected at this size, not a border-rendering bug.
				gen := m.CurrentStep().(*TargetStep).generation
				m.Update(nodesLoadedMsg{generation: gen, nodes: []cluster.NodeDetail{
					{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
					{Name: "homelab-master1", Role: nodetypes.RoleMaster, Ready: true},
					{Name: "homelab-master2", Role: nodetypes.RoleMaster, Ready: true},
					{Name: "homelab-worker0", Role: nodetypes.RoleWorker, Ready: true},
					{Name: "homelab-worker1", Role: nodetypes.RoleWorker, Ready: true},
					{Name: "homelab-worker2", Role: nodetypes.RoleWorker, Ready: true},
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
				gen := m.CurrentStep().(*TargetStep).generation
				m.Update(nodesLoadedMsg{generation: gen, nodes: []cluster.NodeDetail{
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
				// A realistic seed, not the old bare generic string: item 3 of
				// the second-cut safety findings folds the specific gate reason
				// into this same Msg (see errtypes.ClusterError's Msg-only
				// Error() contract), so this golden must show a real reason
				// line, not just the generic refusal. Matches
				// terraform.AssertOnlyChange's real len(changes)!=1 wording
				// verbatim.
				m.Update(dryRunDoneMsg{err: &errtypes.ClusterError{
					Msg: `plan safety gate refused the change: plan gate: expected exactly one change (update of "m.master0") but plan has 2: [update m.master0, delete m.worker2]`,
				}})
			},
		},
		{
			name: "preview_blockers",
			id:   StepIDPreview,
			build: func() (*State, Hooks) {
				// cfg.Cluster.Name matches removeWorkerPlan's Cluster/target
				// fixture ("homelab"); see the "confirm" scenario's comment.
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "homelab"
				return &State{
					Cfg: cfg, Op: node.OpRemove,
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
				// cfg.Cluster.Name matches removeWorkerPlan's Cluster/target
				// fixture ("homelab") — an unmodified config.DefaultConfig()
				// here previously left the footer/trail reading "mycluster"
				// under a "homelab" plan body and type-to-confirm target.
				cfg := config.DefaultConfig()
				cfg.Cluster.Name = "homelab"
				return &State{
					Cfg: cfg, Op: node.OpRemove,
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
			// spinner row and right-aligned durations, m2 still pending;
			// the log tail rides under the checklist (or in the pane on
			// the split tier).
			name: "exec",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), goldenHooks()
			},
			seed: seedExecMidRun,
		},
		{
			// The log window frozen where `l` locked it, header marked so
			// a still tail never reads as a stalled run.
			name: "exec_locked",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), goldenHooks()
			},
			seed: func(m *wizard.Model, st *State) {
				seedExecMidRun(m, st)
				m.Update(tea.KeyPressMsg{Code: logview.KeyLock, Text: "l"})
				m.Update(wizard.FocusChangedMsg{})
			},
		},
		{
			// The filter input mid-typing: the chip leads with the "/" that
			// opened it and counts matches from the first keystroke.
			name: "exec_filtering",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), goldenHooks()
			},
			seed: func(m *wizard.Model, st *State) {
				seedExecMidRun(m, st)
				typeFilter(m, "step-2")
			},
		},
		{
			// The committed filter: only matching rows, and the chip names
			// the pattern it is selecting on.
			name: "exec_filtered",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), goldenHooks()
			},
			seed: func(m *wizard.Model, st *State) {
				seedExecMidRun(m, st)
				typeFilter(m, "step-2")
				m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			},
		},
		{
			// A pattern that matches nothing: the rows go, the chip stays,
			// and the region says why it is empty.
			name: "exec_filter_empty",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), goldenHooks()
			},
			seed: func(m *wizard.Model, st *State) {
				seedExecMidRun(m, st)
				typeFilter(m, "ceph")
			},
		},
		{
			// N walked the window back to the run's one error, which the
			// minimap lane had been marking all along.
			name: "exec_jumped",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), errorEarlyRing()
			},
			seed: func(m *wizard.Model, st *State) {
				seedExecMidRun(m, st)
				m.Update(tea.KeyPressMsg{Code: logview.KeyPrevMatch, Text: "N"})
				m.Update(wizard.FocusChangedMsg{})
			},
		},
		{
			// `f` swapped the log full-screen: the checklist steps aside
			// and the log takes the whole frame, its sink path in the header.
			name: "exec_full",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), goldenHooks()
			},
			seed: func(m *wizard.Model, st *State) {
				seedExecMidRun(m, st)
				m.Update(tea.KeyPressMsg{Code: logview.KeyFull, Text: "f"})
				m.Update(wizard.LayoutChangedMsg{})
			},
		},
		{
			// The graceful-cancel amber line, shown under the headline
			// while the current gate finishes.
			name: "exec_cancel",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), goldenHooks()
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
				return threeMasterState(), goldenHooks()
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
				return st, goldenHooks()
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
				st.Result = errGoldenFailure
				return st, goldenHooks()
			},
		},
		{
			// The full incident report, reached the way a real failure reaches
			// it: the gate checklist frozen where the operation stopped above
			// the card, the run's identity, the next moves, then the evidence.
			name: "done_incident",
			id:   StepIDExec,
			build: func() (*State, Hooks) {
				return threeMasterState(), goldenHooks()
			},
			seed: seedExecFailure,
		},
	}
}

// errGoldenFailure is the backend failure every failure golden renders.
var errGoldenFailure = errors.New("etcd health gate (post-master0) failed: quorum lost")

var lifecycleGoldenSizes = []struct {
	w, h int
	fits bool
}{
	{80, 24, true},
	{100, 30, true},
	{120, 40, true},
	{150, 45, true},
	{180, 48, true},
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
