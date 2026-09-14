package lifecycle

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

type previewPhase int

const (
	previewRunning previewPhase = iota
	previewDone
)

type dryRunDoneMsg struct {
	plan *node.OpPlan
	err  error
}

// Preview action indices, aligned with the selector option order.
const (
	previewActionExecute = iota
	previewActionBack
	previewActionExit
)

// gateGridCellWidth is the fixed column width for one renderGateGrid cell.
const gateGridCellWidth = 28

// PreviewStep runs the real dry-run pass (guards + plan safety gate) and
// renders the informed plan — nodes, terraform actions, health-gate plan,
// and destructive warnings — with the execute/back/exit action selector
// pinned to the help row.
type PreviewStep struct {
	wizard.BaseStep
	st    *State
	hooks Hooks

	phase          previewPhase
	loadingSpinner spinner.Model
	actions        *components.CompactSelector
	exitChosen     bool
}

// NewPreviewStep constructs the plan-preview step.
func NewPreviewStep(st *State, hooks Hooks) *PreviewStep {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	return &PreviewStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(StepIDPreview,
			"plan preview", "review the plan", ""),
		st:             st,
		hooks:          hooks,
		phase:          previewRunning,
		loadingSpinner: sp,
	}
}

// Init re-runs the dry-run on every focus so the plan is always fresh, and
// clears any stale consent from an earlier pass through this screen.
func (s *PreviewStep) Init() tea.Cmd {
	s.phase = previewRunning
	s.exitChosen = false
	s.st.Proceed = false
	s.st.Plan = nil
	s.st.DryRunErr = nil
	run := func() tea.Msg {
		if s.hooks.DryRun == nil {
			return dryRunDoneMsg{}
		}
		plan, err := s.hooks.DryRun(s.st)
		return dryRunDoneMsg{plan: plan, err: err}
	}
	return tea.Batch(s.loadingSpinner.Tick, run)
}

// Update handles dry-run completion, spinner ticks, and action selection.
func (s *PreviewStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case dryRunDoneMsg:
		s.phase = previewDone
		s.st.Plan = msg.plan
		s.st.DryRunErr = msg.err
		if msg.err == nil {
			s.actions = components.NewCompactSelector([]string{
				"execute " + opVerb(s.st.Op),
				"back to parameters",
				"exit without changes",
			})
			s.actions.SetWrap(false)
		}
		return s, nil

	case spinner.TickMsg:
		if s.phase == previewRunning {
			var cmd tea.Cmd
			s.loadingSpinner, cmd = s.loadingSpinner.Update(msg)
			return s, cmd
		}

	case tea.KeyPressMsg:
		if s.phase != previewDone || s.st.DryRunErr != nil || s.actions == nil {
			return s, nil
		}
		if msg.Code == tea.KeyEnter {
			switch s.actions.SelectedIndex() {
			case previewActionExecute:
				s.st.Proceed = true
				return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDPreview} }
			case previewActionBack:
				return s, func() tea.Msg { return wizard.StepBackMsg{} }
			default:
				s.exitChosen = true
				return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDPreview} }
			}
		}
		var cmd tea.Cmd
		s.actions, cmd = s.actions.Update(components.ArrowsAsVertical(msg))
		return s, cmd
	}
	return s, nil
}

// InterceptBack blocks esc while the dry-run is in flight: navigating
// away would orphan a runlock-holding runner and make the next dry-run
// fail on the lock until it finishes.
func (s *PreviewStep) InterceptBack() bool {
	return s.phase == previewRunning
}

// ShouldExitEarly quits the wizard when the operator chose exit-without-
// changes; execute advances to the confirm/exec steps instead.
func (s *PreviewStep) ShouldExitEarly() bool {
	return s.exitChosen
}

// GetSelectedAction reports the wizard action for the early-exit path.
func (s *PreviewStep) GetSelectedAction() wizard.Action {
	return wizard.ActionExit
}

// View renders the spinner, the dry-run failure, or the informed plan.
func (s *PreviewStep) View(width, height int) string {
	s.SetSize(width, height)

	if s.phase == previewRunning {
		return s.loadingSpinner.View() + " running guards and the terraform plan gate (dry-run)..."
	}
	if s.st.DryRunErr != nil {
		errStyle := lipgloss.NewStyle().Foreground(tui.ColorError).Bold(true)
		hintStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500).Italic(true)
		return errStyle.Render("dry-run failed") + "\n\n" +
			lipgloss.NewStyle().Foreground(tui.ColorText).Render(s.st.DryRunErr.Error()) + "\n\n" +
			hintStyle.Render("esc to go back and adjust")
	}
	if s.st.Plan == nil {
		return "no plan"
	}

	st := wizard.NewSectionStyles(width)
	var b strings.Builder

	b.WriteString(wizard.RenderSection(&st, "operation", s.operationEntries()))
	b.WriteString(s.renderNodes(&st, width))
	b.WriteString(s.renderGates(&st, width))

	if s.st.Plan.DestroysData() {
		b.WriteString(s.renderIrreversibleBlock(width))
	}

	return strings.TrimRight(b.String(), "\n")
}

func (s *PreviewStep) operationEntries() []wizard.KVEntry {
	plan := s.st.Plan
	entries := []wizard.KVEntry{
		{Label: "cluster", Value: plan.Cluster},
		{Label: "operation", Value: s.operationLabel()},
	}
	if s.st.Op == node.OpResize {
		current := s.currentRoleMemoryMB()
		if plan.MemoryMB > 0 && plan.MemoryMB != current {
			entries = append(entries, wizard.KVEntry{
				Label: "target memory",
				Value: fmt.Sprintf("%d → %d MiB per node", current, plan.MemoryMB),
			})
		}
		cpuLine := fmt.Sprintf("%d vCPU per node", plan.CPU)
		if plan.CPU <= 0 {
			cpuLine = "unchanged"
		}
		entries = append(entries, wizard.KVEntry{Label: "target cpu", Value: cpuLine})
		if plan.OSDiskGB > 0 {
			entries = append(entries, wizard.KVEntry{
				Label: "target os disk",
				Value: fmt.Sprintf("%d → %d GiB per node", s.currentRoleDiskGB(), plan.OSDiskGB),
			})
		}
		disruption := "each node is drained, then power-cycled (stop→start)"
		if s.st.SkipDrain {
			disruption = "power-cycle without drain (pods restart in place)"
		}
		if s.st.DiskOnly() {
			disruption = "live resize — no drain, no power-cycle"
		}
		entries = append(entries, wizard.KVEntry{Label: sectionDisruption, Value: disruption})
	}
	if s.st.DrainTimeout != "" && !s.st.SkipDrain && s.st.Op != node.OpAdd {
		entries = append(entries, wizard.KVEntry{Label: "drain timeout", Value: s.st.DrainTimeout})
	}
	if s.st.Op == node.OpAdd {
		entries = append(entries, wizard.KVEntry{
			Label: "ignition server",
			Value: "revived for the join window, then torn down",
		})
	}
	return entries
}

// renderNodes renders the "nodes — execution order" section as an aligned
// table, one line per node plus any OSD/ingress warning lines beneath it.
func (s *PreviewStep) renderNodes(st *wizard.SectionStyles, width int) string {
	var b strings.Builder
	b.WriteString(st.Header.Render("nodes — execution order"))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")
	for _, line := range s.renderNodeTable(width) {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

// renderNodeTable renders the plan's nodes as a NODE/ROLE/ADDRESS/ACTION
// table, with each node's OSD/ingress destructive-storage warnings (if any)
// inserted as plain lines directly beneath its row.
func (s *PreviewStep) renderNodeTable(width int) []string {
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	nodes := s.st.Plan.Nodes

	data := make([][]string, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		data[i] = []string{n.Name, string(n.Role), n.TFAddress, string(n.Action)}
	}

	colWidth := max(16, width/3)
	lines := tui.Table([]string{"NODE", "ROLE", "ADDRESS", "ACTION"}, data, tui.TableOptions{MaxColWidth: colWidth})

	out := make([]string, 0, len(lines)+len(nodes))
	out = append(out, lines[0])
	for i := range nodes {
		n := &nodes[i]
		out = append(out, lines[i+1])
		if len(n.OSDs) > 0 {
			out = append(out, warnStyle.Render(fmt.Sprintf("  storage: %d rook-ceph OSD(s) — data disk destroyed", len(n.OSDs))))
		}
		if len(n.Ingress) > 0 {
			out = append(out, warnStyle.Render(fmt.Sprintf("  ingress: %d router pod(s) here", len(n.Ingress))))
		}
	}
	return out
}

// renderGates renders the "gates per node" section as a numbered gate grid
// followed by the plan-safety-gate confirmation line.
func (s *PreviewStep) renderGates(st *wizard.SectionStyles, width int) string {
	okStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	var b strings.Builder
	b.WriteString(st.Header.Render("gates per node"))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")
	gates := GateRows(s.st.Op, s.planRole(), s.st.SkipDrain, diskModeFor(s.st))
	for _, line := range renderGateGrid(gates, width) {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(okStyle.Render(tui.IconSuccess + " plan gate passed — the safety gate allows exactly the listed changes"))
	b.WriteString("\n\n")
	return b.String()
}

// renderGateGrid lays out gates as a column-major numbered grid of fixed
// gateGridCellWidth cells: three columns when width is at least 90, else two.
func renderGateGrid(gates []string, width int) []string {
	if len(gates) == 0 {
		return nil
	}
	cols := 2
	if width >= 90 {
		cols = 3
	}
	rowsPerCol := (len(gates) + cols - 1) / cols
	cell := lipgloss.NewStyle().Width(gateGridCellWidth)

	lines := make([]string, rowsPerCol)
	for r := range lines {
		var row strings.Builder
		for c := 0; c < cols; c++ {
			idx := c*rowsPerCol + r
			if idx >= len(gates) {
				continue
			}
			text := tui.Truncate(fmt.Sprintf("%d %s", idx+1, gates[idx]), gateGridCellWidth)
			row.WriteString(cell.Render(text))
		}
		lines[r] = row.String()
	}
	return lines
}

// renderIrreversibleBlock renders the two-line red-bar warning for a plan
// that destroys a data disk: a bold "irreversible" label line followed by
// the shared render.IrreversibleWarning wrapped to width−2.
func (s *PreviewStep) renderIrreversibleBlock(width int) string {
	barStyle := lipgloss.NewStyle().Foreground(tui.ColorError)
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	bar := barStyle.Render(tui.IconBar)

	var b strings.Builder
	b.WriteString(bar + " " + labelStyle.Render("irreversible"))
	b.WriteString("\n")
	wrapped := lipgloss.Wrap(render.IrreversibleWarning, width-2, "")
	for _, line := range strings.Split(wrapped, "\n") {
		b.WriteString(bar + " " + textStyle.Render(line))
		b.WriteString("\n")
	}
	return b.String()
}

func (s *PreviewStep) operationLabel() string {
	switch s.st.Op {
	case node.OpResize:
		switch {
		case s.st.Scope.Node != "":
			return "resize " + s.st.Scope.Node
		case s.st.Scope.Role == nodetypes.RoleMaster:
			return "resize masters"
		default:
			return "resize workers"
		}
	case node.OpAdd:
		return fmt.Sprintf("add %d worker(s)", max(s.st.Count, 1))
	case node.OpRemove:
		return "remove " + s.st.Target
	default:
		return string(s.st.Op)
	}
}

// planRole prefers the plan's first node role (authoritative) over the scoped role.
func (s *PreviewStep) planRole() nodetypes.NodeRole {
	if len(s.st.Plan.Nodes) > 0 && s.st.Plan.Nodes[0].Role != "" {
		return s.st.Plan.Nodes[0].Role
	}
	if s.st.Scope.Role != "" {
		return s.st.Scope.Role
	}
	return nodetypes.RoleWorker
}

func (s *PreviewStep) currentRoleMemoryMB() int {
	if s.planRole() == nodetypes.RoleMaster {
		return s.st.Cfg.Topology.ControlPlane.MemoryMB
	}
	return s.st.Cfg.Topology.Workers.MemoryMB
}

func (s *PreviewStep) currentRoleDiskGB() int {
	if s.planRole() == nodetypes.RoleMaster {
		return s.st.Cfg.Topology.ControlPlane.DiskGB
	}
	return s.st.Cfg.Topology.Workers.DiskGB
}

func opVerb(op node.Op) string {
	switch op {
	case node.OpResize:
		return "resize"
	case node.OpAdd:
		return "add"
	case node.OpRemove:
		return "removal"
	default:
		return string(op)
	}
}

// SetFocused propagates focus to the action selector.
func (s *PreviewStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if s.actions != nil {
		s.actions.SetFocused(focused)
	}
}

// PinnedFooter renders the action selector inline on the help row, empty
// while the dry-run is running or after it fails (no selector to drive).
func (s *PreviewStep) PinnedFooter(width int) string {
	if s.actions == nil {
		return ""
	}
	// MaxWidth (not tui.Truncate) because ViewInline is already ANSI-styled;
	// lipgloss truncates styled text ANSI-safely, a rune slice would not.
	return lipgloss.NewStyle().MaxWidth(width).Render(s.actions.ViewInline())
}

// ShortHelp returns the preview help bar: with no action selector built
// (still running, or the dry-run failed) it advertises only the keys
// Update actually handles there — esc-back once InterceptBack releases it,
// ctrl+c quit always — never the choose/confirm keys the selector alone
// drives; with a selector, the full action/navigation bar.
func (s *PreviewStep) ShortHelp() []wizard.KeyBinding {
	if s.actions == nil {
		help := []wizard.KeyBinding{}
		if s.phase != previewRunning {
			help = append(help, wizard.KeyBinding{Key: wizard.HelpEsc, Help: wizard.HelpBack})
		}
		return append(help, wizard.KeyBinding{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit})
	}
	return []wizard.KeyBinding{
		{Key: wizard.HelpLeftRight, Help: wizard.HelpChoose},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}
