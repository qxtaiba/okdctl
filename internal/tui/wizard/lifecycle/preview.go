package lifecycle

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
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

// irreversibleWarning is the amber wording shared with render.NodeOpConfirm.
const irreversibleWarning = "irreversible: destroys the listed VM(s) and their data disk; removed data cannot be recovered"

// PreviewStep runs the real dry-run pass (guards + plan safety gate) and
// renders the informed plan — nodes, terraform actions, health-gate plan,
// and destructive warnings — with the execute/back/exit action selector.
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
		s.actions, cmd = s.actions.Update(msg)
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
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	var b strings.Builder

	b.WriteString(wizard.RenderSection(&st, "[1] operation", s.operationEntries()))
	b.WriteString(s.renderNodes(&st, &warnStyle))
	b.WriteString(s.renderGates(&st))

	if s.st.Plan.DestroysData() {
		b.WriteString(warnStyle.Render(irreversibleWarning))
		b.WriteString("\n\n")
	}

	b.WriteString(st.ThickSeparator)
	b.WriteString("\n\n")
	b.WriteString(s.actions.View())
	return b.String()
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

func (s *PreviewStep) renderNodes(st *wizard.SectionStyles, warnStyle *lipgloss.Style) string {
	var b strings.Builder
	b.WriteString(st.Header.Render("[2] nodes — execution order"))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")
	for i := range s.st.Plan.Nodes {
		n := &s.st.Plan.Nodes[i]
		b.WriteString(st.KVPair(n.Name, fmt.Sprintf("%s  %s  [%s]", n.Role, n.TFAddress, n.Action)))
		b.WriteString("\n")
		if len(n.OSDs) > 0 {
			b.WriteString(warnStyle.Render(fmt.Sprintf("  storage: %d rook-ceph OSD(s) — data disk destroyed", len(n.OSDs))))
			b.WriteString("\n")
		}
		if len(n.Ingress) > 0 {
			b.WriteString(warnStyle.Render(fmt.Sprintf("  ingress: %d router pod(s) here", len(n.Ingress))))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func (s *PreviewStep) renderGates(st *wizard.SectionStyles) string {
	okStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	var b strings.Builder
	b.WriteString(st.Header.Render("[3] gates per node"))
	b.WriteString("\n")
	b.WriteString(st.Separator)
	b.WriteString("\n")
	b.WriteString(strings.Join(GateRows(s.st.Op, s.planRole(), s.st.SkipDrain, diskModeFor(s.st)), " → "))
	b.WriteString("\n")
	b.WriteString(okStyle.Render("plan gate: ✓ dry-run passed — the safety gate allows exactly the listed changes"))
	b.WriteString("\n\n")
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

// planRole resolves the role driving the gate table: the plan's first node
// (authoritative), falling back to the scoped role.
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

// ShortHelp returns the preview help bar, or nil while the dry-run runs.
func (s *PreviewStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == previewRunning {
		return nil
	}
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "select action"},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}
