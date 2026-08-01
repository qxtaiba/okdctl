package lifecycle

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func loadedTarget(t *testing.T, st *State, nodes []cluster.NodeDetail) *TargetStep {
	t.Helper()
	s := NewTargetStep(st, Hooks{ListNodes: func() ([]cluster.NodeDetail, error) { return nodes, nil }})
	cmds := s.Init()
	if cmds == nil {
		t.Fatal("Init must fetch nodes")
	}
	updated, _ := s.Update(nodesLoadedMsg{nodes: nodes})
	return updated.(*TargetStep)
}

func TestTargetStepResizeRoleAndNodeChoices(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	nodes := []cluster.NodeDetail{
		{Name: "homelab-worker0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
	}
	s := loadedTarget(t, st, nodes)
	if got := s.selectableCount(); got != 4 { // masters group, workers group, 2 nodes
		t.Fatalf("selectable options = %d, want 4", got)
	}
	if err := s.Apply(nil); err != nil { // first option: masters role
		t.Fatal(err)
	}
	if st.Scope.Role != nodetypes.RoleMaster || st.Scope.Node != "" {
		t.Fatalf("Scope = %+v, want role master", st.Scope)
	}
	if len(st.Nodes) != 2 {
		t.Fatalf("state must retain the live node list, got %d", len(st.Nodes))
	}
}

func TestTargetStepRemoveOnlyTopWorkerSelectable(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpRemove}
	nodes := []cluster.NodeDetail{
		{Name: "homelab-worker0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-worker2", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-worker1", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
	}
	s := loadedTarget(t, st, nodes)
	if got := s.selectableCount(); got != 1 {
		t.Fatalf("selectable options = %d, want 1 (top worker only)", got)
	}
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.Target != "homelab-worker2" {
		t.Fatalf("Target = %q, want homelab-worker2", st.Target)
	}
	if !strings.Contains(s.View(80, 40), "removable only after") {
		t.Error("lower workers must render with the top-down explanation")
	}
}

func TestTargetStepEnterCompletes(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := loadedTarget(t, st, []cluster.NodeDetail{
		{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
	})
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must emit a command")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Fatal("enter must complete the step")
	}
}

func TestTargetStepLoadErrorBlocksCompletion(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := NewTargetStep(st, Hooks{ListNodes: func() ([]cluster.NodeDetail, error) {
		return nil, errors.New("cluster unreachable")
	}})
	_ = s.Init()
	updated, _ := s.Update(nodesLoadedMsg{err: errors.New("cluster unreachable")})
	s = updated.(*TargetStep)
	if !strings.Contains(s.View(80, 40), "cluster unreachable") {
		t.Error("load error must be visible")
	}
	if _, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Error("enter must be inert when the node list failed to load")
	}
}

func TestTargetStepShouldShow(t *testing.T) {
	cfg := config.DefaultConfig()
	for _, tc := range []struct {
		op   node.Op
		res  bool
		want bool
	}{
		{node.OpResize, false, true},
		{node.OpRemove, false, true},
		{node.OpAdd, false, false},
		{node.OpResize, true, false},
	} {
		st := &State{Cfg: cfg, Op: tc.op, Resume: tc.res}
		if got := NewTargetStep(st, Hooks{}).ShouldShow(cfg); got != tc.want {
			t.Errorf("ShouldShow(op=%v resume=%v) = %v, want %v", tc.op, tc.res, got, tc.want)
		}
	}
}

func TestTargetStepTitleFollowsOp(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := NewTargetStep(st, Hooks{})
	if got := s.DisplayTitle(); !strings.Contains(got, "resized") {
		t.Errorf("resize title = %q", got)
	}
	st.Op = node.OpRemove
	if got := s.DisplayTitle(); !strings.Contains(got, "removed") {
		t.Errorf("remove title = %q", got)
	}
}
