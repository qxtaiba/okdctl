package lifecycle

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// previewWith returns a preview step past its dry-run, with the fold-guard
// already armed (as if the wizard had confirmed the plan's last line was
// shown) — the honest default for tests exercising selection/action
// behavior, not the fold-guard itself. See TestPreviewFoldGuard* for the
// disarmed state.
func previewWith(t *testing.T, st *State, plan *node.OpPlan, err error) *PreviewStep {
	t.Helper()
	s := NewPreviewStep(st, Hooks{DryRun: func(*State) (*node.OpPlan, error) { return plan, err }})
	_ = s.Init()
	updated, _ := s.Update(dryRunDoneMsg{plan: plan, err: err})
	ps := updated.(*PreviewStep)
	ps.NotifyViewportAtBottom()
	return ps
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

// TestPreviewDryRunErrorRendersGateReason guards item 3 of the second-cut
// safety findings: the preview used to render only the generic "plan safety
// gate refused the change" (node.Runner's planTargeted now folds the
// specific gate reason into that same Msg — see the ClusterError.Error()
// Msg-only contract in errtypes.go), leaving the operator no more specific
// than done_failure's "etcd health gate (post-master0) failed: quorum lost".
func TestPreviewDryRunErrorRendersGateReason(t *testing.T) {
	st := &State{Cfg: config.DefaultConfig(), Op: node.OpResize}
	// Matches terraform.AssertOnlyChange's real len(changes)!=1 wording
	// (internal/infrastructure/terraform/plangate.go) verbatim, so this
	// fixture can't drift from what runner.go actually produces.
	seeded := &errtypes.ClusterError{
		Msg: `plan safety gate refused the change: plan gate: expected exactly one change (update of "m.master0") but plan has 2: [update m.master0, delete m.worker2]`,
	}
	s := previewWith(t, st, nil, seeded)
	// Wide enough that lipgloss.Wrap never splits a phrase across lines —
	// the wrapping itself is exercised by the preview_error goldens instead.
	out := s.View(200, 40)
	for _, want := range []string{
		"plan safety gate refused the change",
		`expected exactly one change (update of "m.master0")`,
		"but plan has 2:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run failure must show the specific gate reason %q, in:\n%s", want, out)
		}
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

// TestPreviewGateGridUntruncatedAtRealisticWidths guards item 2 of the
// second-cut safety findings: the old fixed 28-column cell truncated "3
// terraform apply (in-place update)" (35 chars) at every width. The column
// width must now be derived from the actual width, checked at both the raw
// 120 the brief's own contract names and 110 — the chrome-adjusted inner
// width a real 120x40 terminal actually hands renderGateGrid (120 minus
// header/border/padding overhead). At 110 the fold-in's remainder
// distribution (37/37/36 instead of leaving 36/36/36 on the floor) is what
// closes the last one-char gap the review caught in the un-redistributed
// version.
func TestPreviewGateGridUntruncatedAtRealisticWidths(t *testing.T) {
	gates := GateRows(node.OpResize, nodetypes.RoleMaster, false, DiskNone)
	longest := 0
	for i, g := range gates {
		if n := len(fmt.Sprintf("%d %s", i+1, g)); n > longest {
			longest = n
		}
	}

	for _, width := range []int{110, 120} {
		rows := renderGateGrid(gates, width)
		for _, row := range rows {
			if strings.Contains(row, "…") {
				t.Errorf("gate label truncated at width %d (longest label is %d chars), row: %q", width, longest, row)
			}
		}
	}
}

// TestPreviewGateGridKeepsGutterOnTruncation guards item 2's collision fix:
// a label truncated to fill its column must still leave a >=2-space gutter
// before the next column's text, so the ellipsis never runs into it. The
// fixture's own gate names are too short to force truncation at any
// wizard-reachable width, so this seeds an oversized label directly.
func TestPreviewGateGridKeepsGutterOnTruncation(t *testing.T) {
	gates := []string{strings.Repeat("x", 60), "short gate"}
	rows := renderGateGrid(gates, 80)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for 2 gates in a 2-column layout, got %d: %v", len(rows), rows)
	}

	row := rows[0]
	ellipsisAt := strings.Index(row, "…")
	if ellipsisAt < 0 {
		t.Fatalf("setup: expected the oversized label to be truncated, row: %q", row)
	}
	after := row[ellipsisAt+len("…"):]
	gutter := len(after) - len(strings.TrimLeft(after, " "))
	if gutter < gateGridGutter {
		t.Errorf("gutter after a truncated label = %d spaces, want >= %d: row %q", gutter, gateGridGutter, row)
	}
	if !strings.HasPrefix(strings.TrimLeft(after, " "), "2 short gate") {
		t.Errorf("second column collided with the truncated first column: row %q", row)
	}
}

// TestPreviewGateGridMinimumOneColumn guards the width floor: a width too
// narrow for even a 2-column layout must still render every gate, one
// column wide, rather than dividing into unreadable slivers.
func TestPreviewGateGridMinimumOneColumn(t *testing.T) {
	gates := []string{"a", "b", "c"}
	rows := renderGateGrid(gates, 10)
	if len(rows) != len(gates) {
		t.Fatalf("expected one row per gate at a floor width, got %d rows: %v", len(rows), rows)
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

// TestPreviewFoldGuardBlocksConfirmUntilBottomShown guards item 1 (S4, the
// campaign's lead item) of the second-cut safety findings: at 80x24 the
// plan-gate line sits below the fold while the pre-selected "execute"
// action would otherwise be immediately confirmable — a destructive default
// must never be actionable before its own safety context. The step has no
// visibility into the viewport's own scroll offset, so it gates on the
// wizard's bottom-reached signal instead of the specific line: the smallest
// mechanism that still guarantees the gate line was displayable.
func TestPreviewFoldGuardBlocksConfirmUntilBottomShown(t *testing.T) {
	tui.SetTerminalWidth(80)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := resizePreviewState()
	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 80, 24)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDPreview})
	st.Scope = node.ResizeScope{Role: nodetypes.RoleMaster}
	m.Update(dryRunDoneMsg{plan: masterResizePlan()})

	frame := tuitest.RenderAt(t, m, 80, 24)
	if strings.Contains(frame, tui.IconActive+" execute resize") {
		t.Fatalf("radio must not render before the gate line has been shown:\n%s", frame)
	}
	if !strings.Contains(frame, "scroll to review the plan") {
		t.Errorf("disarmed footer must show the scroll hint:\n%s", frame)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if st.Proceed {
		t.Fatal("enter must not confirm before the gate line has been shown")
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd}) // scroll to the bottom
	frame = tuitest.RenderAt(t, m, 80, 24)
	if !strings.Contains(frame, tui.IconActive+" execute resize") {
		t.Errorf("radio must arm with execute selected once the bottom has been shown:\n%s", frame)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !st.Proceed {
		t.Error("enter must confirm once the fold-guard has armed")
	}
}

// TestPreviewFoldGuardArmsImmediatelyWhenEverythingFits guards the other
// half of item 1's contract: at 120x40 the whole plan, gate line included,
// renders above the fold, so the radio must arm without requiring any
// scrolling.
func TestPreviewFoldGuardArmsImmediatelyWhenEverythingFits(t *testing.T) {
	tui.SetTerminalWidth(120)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := resizePreviewState()
	m := wizard.NewFlowModel(NewSteps(st, Hooks{}), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 120, 40)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDPreview})
	st.Scope = node.ResizeScope{Role: nodetypes.RoleMaster}
	m.Update(dryRunDoneMsg{plan: masterResizePlan()})

	frame := tuitest.RenderAt(t, m, 120, 40)
	if !strings.Contains(frame, tui.IconActive+" execute resize") {
		t.Errorf("radio must arm immediately when the plan fits without scrolling:\n%s", frame)
	}
}
