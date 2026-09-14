package lifecycle

import (
	"fmt"
	"sort"

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
	// header: the node table's header row, shown above the selector for remove
	// (resize carries its own copy via selector.DropdownHeader instead).
	header string
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
		return "choose the target — worker to remove"
	}
	return "choose the target — nodes to resize"
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
	s.header = ""
	s.blocked = nil
	var opts []components.Option
	var dropdownHeader string

	if s.st.Op == node.OpRemove {
		if len(workers) > 0 {
			top := workers[0]
			blockedAfter := make(map[string]string, len(workers)-1)
			for _, w := range workers[1:] {
				blockedAfter[w.Name] = top.Name
			}
			header, rows := nodeTable(workers, blockedAfter)
			s.header = header

			s.choices = append(s.choices, targetChoice{node: top.Name})
			opts = append(opts, components.Option{
				ID:    top.Name,
				Title: rows[0],
			})
			for i := range workers[1:] {
				s.blocked = append(s.blocked, tui.IconSkip+" "+rows[i+1])
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
		allNodes := make([]cluster.NodeDetail, 0, len(masters)+len(workers))
		allNodes = append(allNodes, masters...)
		allNodes = append(allNodes, workers...)
		header, rows := nodeTable(allNodes, nil)
		dropdownHeader = header
		for i, n := range allNodes {
			s.choices = append(s.choices, targetChoice{node: n.Name})
			opts = append(opts, components.Option{
				ID:         n.Name,
				Title:      rows[i],
				InDropdown: true,
			})
		}
	}

	s.selector = components.NewSelector(opts)
	s.selector.DropdownHeader = dropdownHeader
}

// nodeTable renders nodes as an aligned NODE/ROLE/READY table, pre-styling
// each READY cell as ready, notready, or blocked-after via blockedAfter.
func nodeTable(nodes []cluster.NodeDetail, blockedAfter map[string]string) (header string, rows []string) {
	readyStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	notReadyStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	blockedStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate600)

	data := make([][]string, len(nodes))
	for i, n := range nodes {
		var ready string
		switch {
		case blockedAfter[n.Name] != "":
			ready = blockedStyle.Render(fmt.Sprintf("blocked until %s is removed", blockedAfter[n.Name]))
		case n.Ready:
			ready = readyStyle.Render(tui.IconSuccess + " ready")
		default:
			ready = notReadyStyle.Render("notready")
		}
		data[i] = []string{n.Name, string(n.Role), ready}
	}

	lines := tui.Table([]string{"NODE", "ROLE", "READY"}, data, tui.TableOptions{})
	return lines[0], lines[1:]
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
	if s.header != "" {
		out = "  " + s.header + "\n" + out
	}
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

// ShortHelp returns the step's help bar, back/quit only while loading.
func (s *TargetStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == targetLoading {
		return []wizard.KeyBinding{
			{Key: wizard.HelpEsc, Help: wizard.HelpBack},
			{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
		}
	}
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "select"},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}
