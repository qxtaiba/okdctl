package lifecycle

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// TestSortByIndexUnindexedNodesSortLast pins bug 25: an unindexed node
// sorts after every terraform-indexed one, so remove never offers it as
// the top-of-stack removal target while blocking the real workers.
func TestSortByIndexUnindexedNodesSortLast(t *testing.T) {
	nodes := []cluster.NodeDetail{
		{Name: "custom-gpu-node"},
		{Name: "homelab-worker0"},
		{Name: "homelab-worker1"},
	}

	sortByIndex(nodes, true)

	if got := nodes[len(nodes)-1].Name; got != "custom-gpu-node" {
		t.Fatalf("unindexed node did not sort last: %+v", nodes)
	}
	if nodes[0].Name != "homelab-worker1" || nodes[1].Name != "homelab-worker0" {
		t.Fatalf("descending index order broken: %+v", nodes)
	}
}

func loadedTarget(t *testing.T, st *State, nodes []cluster.NodeDetail) *TargetStep {
	t.Helper()
	s := NewTargetStep(st, Hooks{ListNodes: func() ([]cluster.NodeDetail, error) { return nodes, nil }})
	cmds := s.Init()
	if cmds == nil {
		t.Fatal("Init must fetch nodes")
	}
	updated, _ := s.Update(nodesLoadedMsg{generation: s.generation, nodes: nodes})
	return updated.(*TargetStep)
}

func TestTargetStepResizeRoleAndNodeChoices(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	nodes := []cluster.NodeDetail{
		{Name: "homelab-worker0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
	}
	s := loadedTarget(t, st, nodes)
	if got := len(s.choices); got != 4 { // masters group, workers group, 2 nodes
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
	if got := len(s.choices); got != 1 {
		t.Fatalf("selectable options = %d, want 1 (top worker only)", got)
	}
	if err := s.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if st.Target != "homelab-worker2" {
		t.Fatalf("Target = %q, want homelab-worker2", st.Target)
	}
	if !strings.Contains(s.View(80, 40), "blocked until homelab-worker2 is removed") {
		t.Error("lower workers must render with the top-down explanation")
	}
}

func TestTargetRemoveHeaderAndBlockedRowsShareColumns(t *testing.T) {
	// Node names avoid the substring "worker" so it unambiguously locates
	// the ROLE cell rather than a node name or the blocked-row message.
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpRemove}
	nodes := []cluster.NodeDetail{
		{Name: "homelab-node1", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-node0", Role: nodetypes.RoleWorker, Ready: true},
	}
	s := loadedTarget(t, st, nodes)

	var headerLine, blockedLine string
	for _, line := range strings.Split(tuitest.StripANSI(s.View(80, 40)), "\n") {
		switch {
		case strings.Contains(line, "ROLE"):
			headerLine = line
		case strings.Contains(line, tui.IconSkip):
			blockedLine = line
		}
	}
	if headerLine == "" || blockedLine == "" {
		t.Fatalf("expected a header row and a blocked row, got header=%q blocked=%q", headerLine, blockedLine)
	}

	roleIdx := strings.Index(headerLine, "ROLE")
	workerIdx := strings.Index(blockedLine, "worker")
	if roleIdx < 0 || workerIdx < 0 {
		t.Fatalf("expected ROLE in header and worker in blocked row, got header=%q blocked=%q", headerLine, blockedLine)
	}

	roleCol := lipgloss.Width(headerLine[:roleIdx])
	workerCol := lipgloss.Width(blockedLine[:workerIdx])
	if roleCol != workerCol {
		t.Errorf("ROLE column at %d, blocked row's role cell at %d, want equal", roleCol, workerCol)
	}
}

func TestTargetShortHelpNeverNil(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := NewTargetStep(st, Hooks{})
	if s.ShortHelp() == nil {
		t.Error("ShortHelp must not be nil while loading")
	}
	loaded := loadedTarget(t, st, []cluster.NodeDetail{
		{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
	})
	if loaded.ShortHelp() == nil {
		t.Error("ShortHelp must not be nil once picking")
	}
}

func TestTargetResizeDropdownHasHeader(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	nodes := []cluster.NodeDetail{
		{Name: "homelab-worker0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
	}
	s := loadedTarget(t, st, nodes)
	if s.selector.DropdownHeader == "" {
		t.Fatal("resize dropdown must carry a header")
	}
	if !strings.Contains(tuitest.StripANSI(s.selector.DropdownHeader), "ROLE") {
		t.Errorf("dropdown header = %q, want the table header", s.selector.DropdownHeader)
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
	updated, _ := s.Update(nodesLoadedMsg{generation: s.generation, err: errors.New("cluster unreachable")})
	s = updated.(*TargetStep)
	if !strings.Contains(s.View(80, 40), "cluster unreachable") {
		t.Error("load error must be visible")
	}
	if _, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Error("enter must be inert when the node list failed to load")
	}
}

func TestTargetResizeTallTerminalShowsAllNodesWithoutMoreMarker(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Cluster.Name = "homelab"
	st := &State{Cfg: cfg, Op: node.OpResize}

	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 120, 40)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDTarget})
	gen := m.CurrentStep().(*TargetStep).generation
	m.Update(nodesLoadedMsg{generation: gen, nodes: []cluster.NodeDetail{
		{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "homelab-master1", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "homelab-master2", Role: nodetypes.RoleMaster, Ready: true},
		{Name: "homelab-worker0", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-worker1", Role: nodetypes.RoleWorker, Ready: true},
		{Name: "homelab-worker2", Role: nodetypes.RoleWorker, Ready: true},
	}})

	frame := tuitest.RenderAt(t, m, 120, 40)
	view := tuitest.StripANSI(frame)

	for _, name := range []string{
		"homelab-master0", "homelab-master1", "homelab-master2",
		"homelab-worker0", "homelab-worker1", "homelab-worker2",
	} {
		if !strings.Contains(view, name) {
			t.Errorf("120x40 must show %s without windowing it away, got:\n%s", name, view)
		}
	}
	if strings.Contains(view, "more") {
		t.Errorf("120x40 has room for all 6 nodes, must not show a more-marker:\n%s", view)
	}
	tuitest.AssertFits(t, frame, 120, 40)
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

// TestTargetStep_StaleNodesSuccessCannotOverwriteNewer pins the
// request-identity contract: fake request A is issued, a newer request B is
// issued before A replies (the back-then-re-enter case), B's reply lands
// first, and A's now-stale successful reply must not overwrite it.
func TestTargetStep_StaleNodesSuccessCannotOverwriteNewer(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := NewTargetStep(st, Hooks{ListNodes: func() ([]cluster.NodeDetail, error) { return nil, nil }})

	s.Init()
	genA := s.generation

	s.Init() // re-entry while A is still in flight
	genB := s.generation
	if genB == genA {
		t.Fatal("re-entry must issue a new generation")
	}

	nodesA := []cluster.NodeDetail{{Name: "stale-node", Role: nodetypes.RoleWorker, Ready: true}}
	nodesB := []cluster.NodeDetail{{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true}}

	step, _ := s.Update(nodesLoadedMsg{generation: genB, nodes: nodesB})
	s = step.(*TargetStep)
	_, _ = s.Update(nodesLoadedMsg{generation: genA, nodes: nodesA})

	if len(st.Nodes) != 1 || st.Nodes[0].Name != "homelab-master0" {
		t.Fatalf("stale reply A overwrote newer reply B: st.Nodes = %+v", st.Nodes)
	}
}

// TestTargetStep_StaleNodesErrorCannotOverwriteNewer repeats the above with
// A returning an error instead of a success: the stale failure must not
// replace B's good, newer result.
func TestTargetStep_StaleNodesErrorCannotOverwriteNewer(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := NewTargetStep(st, Hooks{ListNodes: func() ([]cluster.NodeDetail, error) { return nil, nil }})

	s.Init()
	genA := s.generation

	s.Init() // re-entry while A is still in flight
	genB := s.generation

	nodesB := []cluster.NodeDetail{{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true}}
	step, _ := s.Update(nodesLoadedMsg{generation: genB, nodes: nodesB})
	s = step.(*TargetStep)
	step, _ = s.Update(nodesLoadedMsg{generation: genA, err: errors.New("cluster unreachable")})
	s = step.(*TargetStep)

	if s.loadErr != nil {
		t.Fatalf("stale error reply set loadErr = %v, want nil", s.loadErr)
	}
	if len(st.Nodes) != 1 {
		t.Fatalf("stale error reply altered st.Nodes: %+v", st.Nodes)
	}
}

// TestTargetStep_ReentryAfterSuccessReusesCachedNodes pins the
// reuse-on-re-entry decision: once the node list has loaded, going back and
// re-entering the step must not re-issue the fetch.
func TestTargetStep_ReentryAfterSuccessReusesCachedNodes(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	nodes := []cluster.NodeDetail{{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true}}
	s := loadedTarget(t, st, nodes)
	if s.phase != targetPicking || s.loadErr != nil {
		t.Fatalf("phase=%v loadErr=%v after first load, want targetPicking with no error", s.phase, s.loadErr)
	}

	if cmd := s.Init(); cmd != nil {
		t.Fatal("re-entry after a successful load re-issued the fetch, want cached reuse")
	}
	if s.phase != targetPicking {
		t.Fatalf("re-entry after success flipped phase to %v, want it to stay targetPicking", s.phase)
	}
}

// TestTargetStep_ReentryAfterErrorRefetches pins the other half of the
// reuse decision: a failed attempt is never cached, so re-entering after an
// error automatically retries instead of leaving the step stuck.
func TestTargetStep_ReentryAfterErrorRefetches(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	s := NewTargetStep(st, Hooks{ListNodes: func() ([]cluster.NodeDetail, error) {
		return nil, errors.New("cluster unreachable")
	}})

	s.Init()
	genA := s.generation
	step, _ := s.Update(nodesLoadedMsg{generation: genA, err: errors.New("cluster unreachable")})
	s = step.(*TargetStep)
	if s.loadErr == nil {
		t.Fatal("expected a load error")
	}

	cmd := s.Init() // re-entry
	if cmd == nil {
		t.Fatal("re-entry after an error must retry, not get stuck")
	}
	genB := s.generation
	if genB == genA {
		t.Fatal("retry on re-entry must issue a new generation")
	}
	nodes := []cluster.NodeDetail{{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true}}
	step, _ = s.Update(nodesLoadedMsg{generation: genB, nodes: nodes})
	s = step.(*TargetStep)
	if s.loadErr != nil {
		t.Fatalf("loadErr = %v after re-entry's retry succeeded, want nil", s.loadErr)
	}
}

// TestTargetStep_RefreshKeyRefetchesNodes pins the explicit escape hatch
// the reuse decision requires: 'r' while picking re-issues the node-list
// fetch even though a successful result is cached.
func TestTargetStep_RefreshKeyRefetchesNodes(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	nodes := []cluster.NodeDetail{{Name: "homelab-master0", Role: nodetypes.RoleMaster, Ready: true}}
	s := loadedTarget(t, st, nodes)
	genBefore := s.generation

	_, cmd := s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("'r' while picking must re-issue the node-list fetch")
	}
	if s.generation == genBefore {
		t.Fatal("'r' must bump the generation")
	}
}
