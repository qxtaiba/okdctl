package lifecycle

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

func TestConfirmStepGatesOnExactClusterName(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	plan := &node.OpPlan{
		Op: node.OpRemove, Cluster: "homelab",
		Nodes: []node.PlanNode{{Name: "homelab-worker2", Action: terraform.PlanActionDelete}},
	}
	st := &State{Cfg: cfg, Op: node.OpRemove, Plan: plan, Proceed: true}
	s := NewConfirmStep(st)
	_ = s.Init()

	s.input.SetValue("homela")
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("mismatched name must not complete the step")
	}
	s.input.SetValue("homelab")
	_, cmd = s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("matching name must complete")
	}
	if _, ok := cmd().(wizard.StepCompleteMsg); !ok {
		t.Fatal("want StepCompleteMsg")
	}
	if !strings.Contains(s.View(80, 40), "homelab-worker2") {
		t.Error("confirm screen must name the destroyed node")
	}
}

func TestConfirmValidatorFeedback(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	plan := &node.OpPlan{
		Op: node.OpRemove, Cluster: "homelab",
		Nodes: []node.PlanNode{{Name: "homelab-worker2", Action: terraform.PlanActionDelete}},
	}
	st := &State{Cfg: cfg, Op: node.OpRemove, Plan: plan, Proceed: true}
	s := NewConfirmStep(st)
	_ = s.Init()

	for _, r := range "homela" {
		s.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := s.View(80, 40); !strings.Contains(got, `must match "homelab"`) {
		t.Fatalf("View() = %q, want a must-match error", got)
	}

	s.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if got := s.View(80, 40); !strings.Contains(got, "matches — press enter") {
		t.Fatalf("View() = %q, want the matches confirmation", got)
	}
}

func TestConfirmPluralisesNodes(t *testing.T) {
	for _, tc := range []struct {
		names []string
		want  string
	}{
		{[]string{"a", "b"}, "destroys a and b and their data disks"},
		{[]string{"a", "b", "c"}, "destroys a, b, and c and their data disks"},
	} {
		cfg := config.DefaultConfig()
		cfg.Cluster.Name = "homelab"
		nodes := make([]node.PlanNode, len(tc.names))
		for i, name := range tc.names {
			nodes[i] = node.PlanNode{Name: name, Action: terraform.PlanActionDelete}
		}
		plan := &node.OpPlan{Op: node.OpRemove, Cluster: "homelab", Nodes: nodes}
		st := &State{Cfg: cfg, Op: node.OpRemove, Plan: plan, Proceed: true}
		s := NewConfirmStep(st)
		_ = s.Init()

		if got := s.View(120, 40); !strings.Contains(got, tc.want) {
			t.Fatalf("View() = %q, want %q", got, tc.want)
		}
	}
}

func TestConfirmStepShouldShowOnlyForDestructivePlans(t *testing.T) {
	cfg := config.DefaultConfig()
	update := &node.OpPlan{Nodes: []node.PlanNode{{Action: terraform.PlanActionUpdate}}}
	del := &node.OpPlan{Nodes: []node.PlanNode{{Action: terraform.PlanActionDelete}}}
	for i, tc := range []struct {
		st   *State
		want bool
	}{
		{&State{Cfg: cfg, Proceed: true, Plan: del}, true},
		{&State{Cfg: cfg, Proceed: true, Plan: update}, false},
		{&State{Cfg: cfg, Proceed: false, Plan: del}, false},
		{&State{Cfg: cfg, Proceed: true, Plan: nil}, false},
	} {
		if got := NewConfirmStep(tc.st).ShouldShow(cfg); got != tc.want {
			t.Errorf("case %d: ShouldShow = %v, want %v", i, got, tc.want)
		}
	}
}
