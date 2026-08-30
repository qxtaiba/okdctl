package lifecycle

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/cluster"
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

type targetPhase int

const (
	targetLoading targetPhase = iota
	targetPicking
)

type nodesLoadedMsg struct {
	nodes []cluster.NodeDetail
	err   error
}

// targetChoice is one selectable option: a whole role or a single node.
type targetChoice struct {
	role nodetypes.NodeRole
	node string
}

// TargetStep picks which nodes the operation acts on, from the live node
// list: a whole role or a single node for resize, the highest-numbered
// worker for remove. Hidden for add (workers only) and on resume (the
// marker names the target).
type TargetStep struct {
	wizard.BaseStep
	st    *State
	hooks Hooks

	phase          targetPhase
	loadingSpinner spinner.Model
	loadErr        error

	selector *components.Selector
	choices  []targetChoice
	// blocked: remove-ineligible workers, rendered dimmed to teach the top-down constraint.
	blocked []string
}

// NewTargetStep constructs the target-select step.
func NewTargetStep(st *State, hooks Hooks) *TargetStep {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	return &TargetStep{
		BaseStep:       wizard.NewBaseStepWithDisplayTitle(StepIDTarget, "target", "", ""),
		st:             st,
		hooks:          hooks,
		phase:          targetLoading,
		loadingSpinner: sp,
	}
}

// DisplayTitle names the screen for the chosen op; computed at render
// time because the step is constructed before the op screen runs.
func (s *TargetStep) DisplayTitle() string {
	if s.st.Op == node.OpRemove {
		return "which worker should be removed?"
	}
	return "which nodes should be resized?"
}

// ShouldShow hides the step for add (workers only) and on resume.
func (s *TargetStep) ShouldShow(_ *config.Config) bool {
	return (s.st.Op == node.OpResize || s.st.Op == node.OpRemove) && !s.st.Resume
}

// Init kicks off the live node fetch and the loading spinner.
func (s *TargetStep) Init() tea.Cmd {
	s.phase = targetLoading
	s.loadErr = nil
	fetch := func() tea.Msg {
		if s.hooks.ListNodes == nil {
			return nodesLoadedMsg{}
		}
		nodes, err := s.hooks.ListNodes()
		return nodesLoadedMsg{nodes: nodes, err: err}
	}
	return tea.Batch(s.loadingSpinner.Tick, fetch)
}

// Update handles node-list arrival, spinner ticks, selector navigation,
// and enter to confirm.
func (s *TargetStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case nodesLoadedMsg:
		s.phase = targetPicking
		s.loadErr = msg.err
		if msg.err == nil {
			s.st.Nodes = msg.nodes
			s.buildChoices(msg.nodes)
		}
		return s, nil

	case spinner.TickMsg:
		if s.phase == targetLoading {
			var cmd tea.Cmd
			s.loadingSpinner, cmd = s.loadingSpinner.Update(msg)
			return s, cmd
		}

	case tea.KeyPressMsg:
		if s.phase != targetPicking || s.loadErr != nil || s.selector == nil || len(s.choices) == 0 {
			return s, nil
		}
		if msg.Code == tea.KeyEnter {
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDTarget} }
		}
		var cmd tea.Cmd
		s.selector, cmd = s.selector.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *TargetStep) buildChoices(nodes []cluster.NodeDetail) {
	masters := filterRole(nodes, nodetypes.RoleMaster)
	workers := filterRole(nodes, nodetypes.RoleWorker)
	sortByIndex(masters, false)
	sortByIndex(workers, s.st.Op == node.OpRemove)

	s.choices = nil
	s.blocked = nil
	var opts []components.Option

	if s.st.Op == node.OpRemove {
		if len(workers) > 0 {
			top := workers[0]
			s.choices = append(s.choices, targetChoice{node: top.Name})
			opts = append(opts, components.Option{
				ID:    top.Name,
				Title: nodeLine(top),
			})
			after := []string{top.Name}
			for _, w := range workers[1:] {
				s.blocked = append(s.blocked,
					fmt.Sprintf("○ %s      removable only after %s (top-down)", w.Name, strings.Join(after, ", ")))
				after = append(after, w.Name)
			}
		}
	} else {
		if len(masters) > 0 {
			s.choices = append(s.choices, targetChoice{role: nodetypes.RoleMaster})
			opts = append(opts, components.Option{
				ID:          "masters",
				Title:       fmt.Sprintf("masters — all %d control-plane nodes", len(masters)),
				Description: "rolled one at a time; etcd-gated before and after every node",
			})
		}
		if len(workers) > 0 {
			s.choices = append(s.choices, targetChoice{role: nodetypes.RoleWorker})
			opts = append(opts, components.Option{
				ID:          "workers",
				Title:       fmt.Sprintf("workers — all %d worker nodes", len(workers)),
				Description: "rolled one at a time; no etcd gate",
			})
		}
		for _, n := range append(masters, workers...) {
			s.choices = append(s.choices, targetChoice{node: n.Name})
			opts = append(opts, components.Option{
				ID:         n.Name,
				Title:      nodeLine(n),
				InDropdown: true,
			})
		}
	}

	s.selector = components.NewSelector(opts)
}

func nodeLine(n cluster.NodeDetail) string {
	ready := "ready"
	if !n.Ready {
		ready = "notready"
	}
	return fmt.Sprintf("%-22s %-8s %s", n.Name, n.Role, ready)
}

func filterRole(nodes []cluster.NodeDetail, role nodetypes.NodeRole) []cluster.NodeDetail {
	var out []cluster.NodeDetail
	for _, n := range nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

// sortByIndex orders by terraform index (desc for remove's top-down list);
// unindexed nodes sort last.
func sortByIndex(nodes []cluster.NodeDetail, descending bool) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, aok := cluster.NodeIndex(nodes[i].Name)
		b, bok := cluster.NodeIndex(nodes[j].Name)
		if !aok || !bok {
			return bok
		}
		if descending {
			return a > b
		}
		return a < b
	})
}

// View renders the spinner, a load error, or the target selector plus any
// blocked-worker lines.
func (s *TargetStep) View(width, height int) string {
	s.SetSize(width, height)

	if s.phase == targetLoading {
		return s.loadingSpinner.View() + " listing cluster nodes..."
	}
	if s.loadErr != nil {
		warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
		hintStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500).Italic(true)
		return warnStyle.Render("list nodes: "+s.loadErr.Error()) + "\n\n" +
			hintStyle.Render("esc to go back")
	}
	if s.selector == nil || len(s.choices) == 0 {
		return lipgloss.NewStyle().Foreground(tui.ColorWarning).Render("no eligible nodes found")
	}

	out := s.selector.View()
	if len(s.blocked) > 0 {
		dim := lipgloss.NewStyle().Foreground(tui.ColorSlate600)
		for _, line := range s.blocked {
			out += "\n" + dim.Render(line)
		}
	}
	return out
}

// Apply writes the selected target into the shared state.
func (s *TargetStep) Apply(_ *config.Config) error {
	if s.selector == nil || len(s.choices) == 0 {
		return nil
	}
	idx := s.selector.SelectedIndex()
	if idx < 0 || idx >= len(s.choices) {
		idx = 0
	}
	c := s.choices[idx]
	switch s.st.Op {
	case node.OpRemove:
		s.st.Target = c.node
	default:
		s.st.Scope = node.ResizeScope{Role: c.role, Node: c.node}
	}
	return nil
}

// SetFocused propagates focus to the selector.
func (s *TargetStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if s.selector != nil {
		s.selector.SetFocused(focused)
	}
}

// ShortHelp returns the step's help bar, or nil while loading.
func (s *TargetStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == targetLoading {
		return nil
	}
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "select"},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}
