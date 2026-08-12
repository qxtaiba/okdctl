package lifecycle

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func previewWith(t *testing.T, st *State, plan *node.OpPlan, err error) *PreviewStep {
	t.Helper()
	s := NewPreviewStep(st, Hooks{DryRun: func(*State) (*node.OpPlan, error) { return plan, err }})
	_ = s.Init()
	updated, _ := s.Update(dryRunDoneMsg{plan: plan, err: err})
	return updated.(*PreviewStep)
}

func pressActionDown(s *PreviewStep) { _, _ = s.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) }

func masterResizePlan() *node.OpPlan {
	return &node.OpPlan{
		Op: node.OpResize, Cluster: "homelab", MemoryMB: 24576,
		Nodes: []node.PlanNode{{
			Name: "homelab-master0", Role: nodetypes.RoleMaster,
			TFAddress: "m.master[0]", Action: terraform.PlanActionUpdate,
		}},
	}
}

func TestPreviewExecuteSetsProceedAndCompletes(t *testing.T) {
	st := &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize,
		Scope: node.ResizeScope{Role: nodetypes.RoleMaster},
	}
	s := previewWith(t, st, masterResizePlan(), nil)
	if st.Plan == nil {
		t.Fatal("dry-run plan must be captured into state")
	}
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // execute is first action
	if cmd == nil {
		t.Fatal("execute must emit a command")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Fatal("execute must emit StepCompleteMsg")
	}
	if !st.Proceed {
		t.Error("execute must set Proceed")
	}
	if s.ShouldExitEarly() {
		t.Error("execute must not early-exit")
	}
}

func TestPreviewExitWithoutChanges(t *testing.T) {
	st := &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize,
		Scope: node.ResizeScope{Role: nodetypes.RoleMaster},
	}
	s := previewWith(t, st, masterResizePlan(), nil)
	pressActionDown(s) // execute -> back
	pressActionDown(s) // back -> exit
	_, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if st.Proceed {
		t.Error("exit must not set Proceed")
	}
	if !s.ShouldExitEarly() || s.GetSelectedAction() != wizard.ActionExit {
		t.Error("exit must early-exit the wizard with ActionExit")
	}
}

func TestPreviewBackReturnsToParameters(t *testing.T) {
	st := &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize,
		Scope: node.ResizeScope{Role: nodetypes.RoleMaster},
	}
	s := previewWith(t, st, masterResizePlan(), nil)
	pressActionDown(s) // execute -> back
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("back must emit a command")
	}
	if _, ok := cmd().(wizard.StepBackMsg); !ok {
		t.Fatal("back must emit StepBackMsg")
	}
}

func TestPreviewDiskOnlyEntries(t *testing.T) {
	st := &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize, OSDiskGB: 100,
		Scope: node.ResizeScope{Role: nodetypes.RoleMaster},
	}
	plan := &node.OpPlan{
		Op: node.OpResize, Cluster: "homelab", OSDiskGB: 100,
		Nodes: []node.PlanNode{{
			Name: "homelab-master0", Role: nodetypes.RoleMaster,
			TFAddress: "m.master[0]", Action: terraform.PlanActionUpdate,
		}},
	}
	s := previewWith(t, st, plan, nil)
	out := s.View(90, 60)
	for _, want := range []string{"target os disk", "live resize — no drain, no power-cycle"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q:\n%s", want, out)
		}
	}
}

func TestPreviewRendersPlanGatesAndWarnings(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpRemove, Target: "homelab-worker2"}
	plan := &node.OpPlan{
		Op: node.OpRemove, Cluster: "homelab",
		Nodes: []node.PlanNode{{
			Name: "homelab-worker2", Role: nodetypes.RoleWorker,
			TFAddress: "m.worker[2]", Action: terraform.PlanActionDelete,
			OSDs: []string{"osd.1"}, Ingress: []string{"router-a"},
		}},
	}
	s := previewWith(t, st, plan, nil)
	out := s.View(90, 60)
	for _, want := range []string{
		"homelab-worker2", "m.worker[2]", "cordon + drain",
		"irreversible", "rook-ceph", "router pod",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q:\n%s", want, out)
		}
	}
}

func TestPreviewDryRunErrorRendered(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := previewWith(t, st, nil, errors.New("plan safety gate refused the change"))
	if !strings.Contains(s.View(90, 40), "plan safety gate refused") {
		t.Error("dry-run failure must be visible in the view")
	}
	if _, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Error("enter must be inert after a failed dry-run")
	}
}

func TestPreviewBlocksEscWhileDryRunInFlight(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := NewPreviewStep(st, Hooks{})
	_ = s.Init() // running phase
	if !s.InterceptBack() {
		t.Error("esc must be blocked while the dry-run holds the run lock")
	}
	updated, _ := s.Update(dryRunDoneMsg{plan: masterResizePlan()})
	if updated.(*PreviewStep).InterceptBack() {
		t.Error("esc must work again once the dry-run finished")
	}
}
