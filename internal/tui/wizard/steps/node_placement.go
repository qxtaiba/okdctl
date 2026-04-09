package steps

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/components"
)

type placementPhase int

const (
	phaseDiscovering placementPhase = iota
	phasePlacing
)

// nodesDiscoveredMsg is sent when the async Proxmox node fetch completes.
type nodesDiscoveredMsg struct {
	nodes []proxmoxNode
	err   error
}

type vmAssignment struct {
	name     string
	selector *components.CompactSelector
}

type NodePlacementStep struct {
	wizard.BaseStep

	cfg   *config.Config
	phase placementPhase

	// discovery
	loadingSpinner spinner.Model
	discovered     []proxmoxNode
	discoveryErr   error

	// "available nodes" text input (manual fallback / edit)
	nodesInput *components.InputField

	// per-VM selectors
	masters []vmAssignment
	workers []vmAssignment

	// navigation: 0 = nodesInput, 1..N = VM selectors
	focusIndex int
	totalSlots int

	availableNodes []string
	lastNodesValue string
}

func NewNodePlacementStep() (*NodePlacementStep, *NodePlacementStep) {
	nodesInput := components.NewInputField("available nodes", "pve1, pve2, pve3")
	nodesInput.Help = "comma-separated list of proxmox node names"

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	s := &NodePlacementStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDNodePlacement,
			"node placement",
			"assign vms to proxmox nodes",
			"configure which proxmox node each vm runs on",
		),
		nodesInput:     nodesInput,
		loadingSpinner: sp,
		phase:          phaseDiscovering,
		focusIndex:     0,
		totalSlots:     1,
	}
	return s, s
}

func (s *NodePlacementStep) ShouldShow(cfg *config.Config) bool {
	if cfg.Provider.Type != config.ProviderProxmox {
		return false
	}
	s.cfg = cfg
	return true
}

func (s *NodePlacementStep) Init() tea.Cmd {
	if s.cfg == nil || s.cfg.Provider.Proxmox == nil {
		return nil
	}

	s.phase = phaseDiscovering
	return tea.Batch(
		s.loadingSpinner.Tick,
		s.fetchNodes,
	)
}

func (s *NodePlacementStep) fetchNodes() tea.Msg {
	nodes, err := discoverProxmoxNodes(s.cfg)
	return nodesDiscoveredMsg{nodes: nodes, err: err}
}

func (s *NodePlacementStep) buildSelectors(availableNodes []string) {
	defaultNode := s.cfg.Provider.Proxmox.Node
	if defaultNode == "" && len(availableNodes) > 0 {
		defaultNode = availableNodes[0]
	}

	s.availableNodes = availableNodes
	s.nodesInput.SetDefault(strings.Join(availableNodes, ", "))
	s.lastNodesValue = s.nodesInput.Value()

	s.masters = buildVMAssignments(
		s.cfg.Cluster.Name, "master",
		s.cfg.Topology.ControlPlane.Count,
		availableNodes, s.cfg.Provider.Proxmox.MasterNodes, defaultNode,
	)
	s.workers = buildVMAssignments(
		s.cfg.Cluster.Name, "worker",
		s.cfg.Topology.Workers.Count,
		availableNodes, s.cfg.Provider.Proxmox.WorkerNodes, defaultNode,
	)

	s.totalSlots = 1 + len(s.masters) + len(s.workers)
	s.focusIndex = 0
}

// resolveAvailableNodes collects unique node names from config when
// discovery fails or returns nothing.
func (s *NodePlacementStep) resolveAvailableNodes() []string {
	// If the user already typed something, use that
	if typed := parseNodeList(s.nodesInput.Value()); len(typed) > 0 {
		return typed
	}

	defaultNode := s.cfg.Provider.Proxmox.Node
	if defaultNode == "" {
		defaultNode = "pve"
	}

	seen := map[string]bool{defaultNode: true}
	ordered := []string{defaultNode}
	for _, n := range s.cfg.Provider.Proxmox.MasterNodes {
		if n != "" && !seen[n] {
			seen[n] = true
			ordered = append(ordered, n)
		}
	}
	for _, n := range s.cfg.Provider.Proxmox.WorkerNodes {
		if n != "" && !seen[n] {
			seen[n] = true
			ordered = append(ordered, n)
		}
	}
	return ordered
}

func buildVMAssignments(clusterName, role string, count int, nodes, configured []string, defaultNode string) []vmAssignment {
	assignments := make([]vmAssignment, count)
	for i := range count {
		name := fmt.Sprintf("%s-%s%d", clusterName, role, i)
		sel := components.NewCompactSelector(nodes)
		sel.SetFocused(false)

		target := defaultNode
		if i < len(configured) && configured[i] != "" {
			target = configured[i]
		}
		for j, n := range nodes {
			if n == target {
				sel.SetSelected(j)
				break
			}
		}

		assignments[i] = vmAssignment{name: name, selector: sel}
	}
	return assignments
}

func (s *NodePlacementStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case nodesDiscoveredMsg:
		s.discovered = msg.nodes
		s.discoveryErr = msg.err
		s.phase = phasePlacing

		var nodeNames []string
		if msg.err == nil && len(msg.nodes) > 0 {
			for _, n := range msg.nodes {
				nodeNames = append(nodeNames, n.Name)
			}
		} else {
			nodeNames = s.resolveAvailableNodes()
		}

		s.buildSelectors(nodeNames)
		return s, s.updateFocus()

	case spinner.TickMsg:
		if s.phase == phaseDiscovering {
			var cmd tea.Cmd
			s.loadingSpinner, cmd = s.loadingSpinner.Update(msg)
			return s, cmd
		}

	case tea.KeyMsg:
		if s.phase != phasePlacing {
			return s, nil
		}
		return s.handleKey(msg)
	}

	return s, nil
}

func (s *NodePlacementStep) handleKey(msg tea.KeyMsg) (wizard.WizardStep, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		return s, func() tea.Msg {
			return wizard.StepCompleteMsg{StepID: s.ID()}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		if s.focusIndex == 0 {
			s.nodesInput.Blur()
			s.syncSelectorsFromInput()
		}
		s.focusIndex++
		if s.focusIndex >= s.totalSlots {
			s.focusIndex = s.totalSlots - 1
		}
		return s, s.updateFocus()

	case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
		s.focusIndex--
		if s.focusIndex < 0 {
			s.focusIndex = 0
		}
		return s, s.updateFocus()

	case key.Matches(msg, key.NewBinding(key.WithKeys("down"))):
		if s.focusIndex == 0 {
			s.nodesInput.Blur()
			s.syncSelectorsFromInput()
			s.focusIndex = 1
			if s.focusIndex >= s.totalSlots {
				s.focusIndex = s.totalSlots - 1
			}
			return s, s.updateFocus()
		}
		s.focusIndex++
		if s.focusIndex >= s.totalSlots {
			s.focusIndex = s.totalSlots - 1
		}
		return s, s.updateFocus()

	case key.Matches(msg, key.NewBinding(key.WithKeys("up"))):
		if s.focusIndex <= 1 {
			s.focusIndex = 0
			return s, s.updateFocus()
		}
		s.focusIndex--
		return s, s.updateFocus()

	case key.Matches(msg, key.NewBinding(key.WithKeys("left", "right"))):
		if s.focusIndex > 0 {
			sel := s.selectorAt(s.focusIndex)
			if sel != nil && sel.Len() > 1 {
				if key.Matches(msg, key.NewBinding(key.WithKeys("left"))) {
					idx := sel.SelectedIndex() - 1
					if idx < 0 {
						idx = sel.Len() - 1
					}
					sel.SetSelected(idx)
				} else {
					idx := sel.SelectedIndex() + 1
					if idx >= sel.Len() {
						idx = 0
					}
					sel.SetSelected(idx)
				}
			}
			return s, nil
		}
	}

	// Pass remaining keys to nodesInput when focused
	if s.focusIndex == 0 {
		var cmd tea.Cmd
		s.nodesInput, cmd = s.nodesInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

func (s *NodePlacementStep) syncSelectorsFromInput() {
	current := s.nodesInput.Value()
	if current == s.lastNodesValue {
		return
	}
	s.lastNodesValue = current

	nodes := parseNodeList(current)
	if len(nodes) == 0 {
		return
	}
	s.availableNodes = nodes

	updateAssignmentOptions(s.masters, nodes)
	updateAssignmentOptions(s.workers, nodes)
}

func updateAssignmentOptions(assignments []vmAssignment, nodes []string) {
	for i := range assignments {
		current := assignments[i].selector.Selected()
		assignments[i].selector.SetOptions(nodes)
		for j, n := range nodes {
			if n == current {
				assignments[i].selector.SetSelected(j)
				break
			}
		}
	}
}

func (s *NodePlacementStep) selectorAt(index int) *components.CompactSelector {
	vmIndex := index - 1
	if vmIndex < len(s.masters) {
		return s.masters[vmIndex].selector
	}
	vmIndex -= len(s.masters)
	if vmIndex < len(s.workers) {
		return s.workers[vmIndex].selector
	}
	return nil
}

func (s *NodePlacementStep) updateFocus() tea.Cmd {
	s.nodesInput.Blur()
	for i := range s.masters {
		s.masters[i].selector.SetFocused(false)
	}
	for i := range s.workers {
		s.workers[i].selector.SetFocused(false)
	}

	if s.focusIndex == 0 {
		return s.nodesInput.Focus()
	}

	sel := s.selectorAt(s.focusIndex)
	if sel != nil {
		sel.SetFocused(true)
	}
	return func() tea.Msg {
		return wizard.FocusChangedMsg{
			FieldIndex:  s.focusIndex,
			TotalFields: s.totalSlots,
		}
	}
}

func (s *NodePlacementStep) View(width, height int) string {
	s.SetSize(width, height)

	headerStyle := lipgloss.NewStyle().
		Foreground(tui.ColorCyan500).
		Bold(true)

	// Loading phase
	if s.phase == phaseDiscovering {
		return s.loadingSpinner.View() + " discovering proxmox nodes..."
	}

	s.nodesInput.SetWidth(width - 4)

	separator := lipgloss.NewStyle().
		Foreground(tui.ColorSlate700).
		Render(strings.Repeat("┄", width-4))

	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400).Width(20)
	activeStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	successStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)

	var b strings.Builder

	// Discovery result banner
	if s.discoveryErr != nil {
		b.WriteString(warnStyle.Render("  could not auto-discover nodes — enter them manually below"))
		b.WriteString("\n\n")
	} else if len(s.discovered) > 0 {
		parts := make([]string, len(s.discovered))
		for i, n := range s.discovered {
			info := n.Name
			if n.CPUs > 0 || n.MemGB > 0 {
				info += fmt.Sprintf(" (%d cpu, %d gb)", n.CPUs, n.MemGB)
			}
			if n.Status != "online" {
				info += " [" + n.Status + "]"
			}
			parts[i] = info
		}
		b.WriteString(successStyle.Render("  discovered: "+strings.Join(parts, ", ")))
		b.WriteString("\n\n")
	}

	// Available nodes input
	b.WriteString(headerStyle.Render("available proxmox nodes"))
	b.WriteString("\n")
	b.WriteString(separator)
	b.WriteString("\n")
	b.WriteString(s.nodesInput.View())
	b.WriteString("\n\n")

	renderSection := func(title string, assignments []vmAssignment, baseIndex int) {
		if len(assignments) == 0 {
			return
		}
		b.WriteString(headerStyle.Render(title))
		b.WriteString("\n")
		b.WriteString(separator)
		b.WriteString("\n")
		for i, a := range assignments {
			globalIdx := baseIndex + i
			isFocused := s.focusIndex == globalIdx

			label := labelStyle.Render(a.name)
			node := a.selector.Selected()
			if isFocused {
				b.WriteString(label + activeStyle.Render("▸ "+node+" ◂"))
				if len(s.availableNodes) > 1 {
					b.WriteString(dimStyle.Render("  ← →"))
				}
			} else {
				b.WriteString(label + dimStyle.Render(node))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	renderSection("control plane placement", s.masters, 1)
	renderSection("worker placement", s.workers, 1+len(s.masters))

	return b.String()
}

func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}

	masterNodes := make([]string, len(s.masters))
	for i, m := range s.masters {
		masterNodes[i] = m.selector.Selected()
	}
	workerNodes := make([]string, len(s.workers))
	for i, w := range s.workers {
		workerNodes[i] = w.selector.Selected()
	}

	cfg.Provider.Proxmox.MasterNodes = masterNodes
	cfg.Provider.Proxmox.WorkerNodes = workerNodes
	return nil
}

func (s *NodePlacementStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused && s.phase == phasePlacing {
		s.updateFocus()
	} else if !focused {
		s.nodesInput.Blur()
		for i := range s.masters {
			s.masters[i].selector.SetFocused(false)
		}
		for i := range s.workers {
			s.workers[i].selector.SetFocused(false)
		}
	}
}

func (s *NodePlacementStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == phaseDiscovering {
		return nil
	}
	if s.focusIndex == 0 {
		return []wizard.KeyBinding{
			{Key: "tab/↓", Help: "next field"},
			{Key: "enter", Help: "confirm"},
			{Key: "esc", Help: "back"},
		}
	}
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "select vm"},
		{Key: "← →", Help: "change node"},
		{Key: "enter", Help: "confirm"},
		{Key: "esc", Help: "back"},
	}
}

func parseNodeList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var nodes []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			nodes = append(nodes, p)
		}
	}
	return nodes
}
