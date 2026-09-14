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
	"github.com/qxtaiba/okdctl/internal/tui"
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

func resizePreviewState() *State {
	return &State{
		Cfg: config.DefaultConfig(), Op: node.OpResize,
		Scope: node.ResizeScope{Role: nodetypes.RoleMaster},
	}
}

func TestPreviewExecuteSetsProceedAndCompletes(t *testing.T) {
	st := resizePreviewState()
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
	st := resizePreviewState()
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
	s := previewWith(t, resizePreviewState(), masterResizePlan(), nil)
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

func removePreviewPlan() *node.OpPlan {
	return &node.OpPlan{
		Op: node.OpRemove, Cluster: "homelab",
		Nodes: []node.PlanNode{{
			Name: "homelab-worker2", Role: nodetypes.RoleWorker,
			TFAddress: "m.worker[2]", Action: terraform.PlanActionDelete,
		}},
	}
}

func TestPreviewPinnedFooterFollowsSelection(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpRemove, Target: "homelab-worker2"}
	s := previewWith(t, st, removePreviewPlan(), nil)

	footer := s.PinnedFooter(100)
	executeAt := strings.Index(footer, tui.IconActive+" execute removal")
	backAt := strings.Index(footer, tui.IconPending+" back to parameters")
	if executeAt < 0 || backAt < 0 || executeAt > backAt {
		t.Errorf("pinned footer must lead with the selected action, got %q", footer)
	}

	pressActionDown(s)
	footer = s.PinnedFooter(100)
	if !strings.Contains(footer, tui.IconActive+" back to parameters") {
		t.Errorf("pinned footer must track the selection after moving down, got %q", footer)
	}

	running := NewPreviewStep(&State{Cfg: config.DefaultConfig(), Op: node.OpResize}, Hooks{})
	_ = running.Init()
	if got := running.PinnedFooter(100); got != "" {
		t.Errorf("pinned footer must be empty while the dry-run runs, got %q", got)
	}
}

func TestPreviewBodyHasNoActionRadio(t *testing.T) {
	s := previewWith(t, resizePreviewState(), masterResizePlan(), nil)
	out := s.View(90, 60)
	for _, unwanted := range []string{"execute resize", "back to parameters", "exit without changes"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("preview body must not render the action radio, found %q in:\n%s", unwanted, out)
		}
	}
}

func TestPreviewGateGridColumns(t *testing.T) {
	gates := GateRows(node.OpResize, nodetypes.RoleMaster, false, DiskNone)
	if len(gates) != 7 {
		t.Fatalf("expected 7 master resize gates, got %d: %v", len(gates), gates)
	}

	wide := renderGateGrid(gates, 100)
	if len(wide) != 3 {
		t.Fatalf("expected 3 rows at width 100, got %d:\n%s", len(wide), strings.Join(wide, "\n"))
	}
	if !strings.Contains(wide[0], "1 etcd health gate (pre)") || !strings.Contains(wide[0], "4 power-cycle vm") {
		t.Errorf("row 1 at width 100 must carry columns 1 and 4, got %q", wide[0])
	}

	narrow := renderGateGrid(gates, 80)
	if len(narrow) != 4 {
		t.Fatalf("expected 4 rows at width 80, got %d:\n%s", len(narrow), strings.Join(narrow, "\n"))
	}
}

func TestPreviewShortHelpNeverNil(t *testing.T) {
	running := NewPreviewStep(&State{Cfg: config.DefaultConfig(), Op: node.OpResize}, Hooks{})
	_ = running.Init()
	help := running.ShortHelp()
	if len(help) != 1 || help[0].Key != wizard.HelpCtrlC || help[0].Help != wizard.HelpQuit {
		t.Errorf("running ShortHelp must be ctrl+c quit only, got %+v", help)
	}

	done := previewWith(t, resizePreviewState(), masterResizePlan(), nil)
	help = done.ShortHelp()
	want := []wizard.KeyBinding{
		{Key: wizard.HelpLeftRight, Help: wizard.HelpChoose},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
	if len(help) != len(want) {
		t.Fatalf("done ShortHelp has %d bindings, want %d: %+v", len(help), len(want), help)
	}
	for i := range want {
		if help[i] != want[i] {
			t.Errorf("done ShortHelp[%d] = %+v, want %+v", i, help[i], want[i])
		}
	}
}

func TestPreviewShortHelpOnDryRunErrorOmitsSelectorKeys(t *testing.T) {
	failed := previewWith(t, resizePreviewState(), nil, errors.New("plan safety gate refused the change"))
	help := failed.ShortHelp()
	want := []wizard.KeyBinding{
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
	if len(help) != len(want) {
		t.Fatalf("error ShortHelp has %d bindings, want %d: %+v", len(help), len(want), help)
	}
	for i := range want {
		if help[i] != want[i] {
			t.Errorf("error ShortHelp[%d] = %+v, want %+v", i, help[i], want[i])
		}
	}
}

func TestPreviewIrreversibleBlockTwoLines(t *testing.T) {
	s := previewWith(t, &State{Cfg: config.DefaultConfig(), Op: node.OpRemove, Target: "homelab-worker2"},
		removePreviewPlan(), nil)
	out := s.View(90, 60)

	var barLines int
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, tui.IconBar) {
			barLines++
		}
	}
	if barLines != 2 {
		t.Errorf("irreversible block must render as exactly two bar-prefixed lines, got %d in:\n%s", barLines, out)
	}
}
