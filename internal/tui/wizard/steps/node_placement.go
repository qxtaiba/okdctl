package steps

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard/components"
)

type vmAssignment struct {
	name     string
	selector *components.CompactSelector
}

type NodePlacementStep struct {
	wizard.BaseStep

	cfg *config.Config

	nodesInput *components.InputField

	masters []vmAssignment
	workers []vmAssignment

	// navigation: 0 = nodesInput, 1..N = VM selectors
	focusIndex int
	totalSlots int

	availableNodes []string
	lastNodesValue string // track input changes to avoid needless rebuilds
}

func NewNodePlacementStep() (*NodePlacementStep, *NodePlacementStep) {
	nodesInput := components.NewInputField("available nodes", "pve1, pve2, pve3")
	nodesInput.Help = "comma-separated list of proxmox node names"

	s := &NodePlacementStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDNodePlacement,
			"node placement",
			"assign vms to proxmox nodes",
			"configure which proxmox node each vm runs on",
		),
		nodesInput: nodesInput,
		focusIndex: 0,
		totalSlots: 1,
	}
	return s, s
}

// ShouldShow captures the config reference. Selectors are built in Init only.
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

	defaultNode := s.cfg.Provider.Proxmox.Node
	if defaultNode == "" {
		defaultNode = "pve"
	}

	// Determine available nodes: from existing config, input, or default
	nodes := s.resolveAvailableNodes(defaultNode)
	s.availableNodes = nodes
	s.nodesInput.SetDefault(strings.Join(nodes, ", "))
	s.lastNodesValue = s.nodesInput.Value()

	// Build selectors
	s.masters = buildVMAssignments(
		s.cfg.Cluster.Name, "master",
		s.cfg.Topology.ControlPlane.Count,
		nodes, s.cfg.Provider.Proxmox.MasterNodes, defaultNode,
	)
	s.workers = buildVMAssignments(
		s.cfg.Cluster.Name, "worker",
		s.cfg.Topology.Workers.Count,
		nodes, s.cfg.Provider.Proxmox.WorkerNodes, defaultNode,
	)

	s.totalSlots = 1 + len(s.masters) + len(s.workers)
	s.focusIndex = 0
	return s.updateFocus()
}

// resolveAvailableNodes collects unique node names from the config's
// MasterNodes/WorkerNodes and the default node. This seeds the input field
// so the user sees all nodes already in use.
func (s *NodePlacementStep) resolveAvailableNodes(defaultNode string) []string {
	// If the user already typed something, use that
	if typed := parseNodeList(s.nodesInput.Value()); len(typed) > 0 {
		return typed
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
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return s, func() tea.Msg {
				return wizard.StepCompleteMsg{StepID: s.ID()}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			// Tab always moves forward (never cycles nodes)
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
			// Shift+tab always moves backward
			s.focusIndex--
			if s.focusIndex < 0 {
				s.focusIndex = 0
			}
			return s, s.updateFocus()

		case key.Matches(msg, key.NewBinding(key.WithKeys("down"))):
			if s.focusIndex == 0 {
				// In text input, down moves to first VM
				s.nodesInput.Blur()
				s.syncSelectorsFromInput()
				s.focusIndex = 1
				if s.focusIndex >= s.totalSlots {
					s.focusIndex = s.totalSlots - 1
				}
				return s, s.updateFocus()
			}
			// On a VM selector, down moves to next VM
			s.focusIndex++
			if s.focusIndex >= s.totalSlots {
				s.focusIndex = s.totalSlots - 1
			}
			return s, s.updateFocus()

		case key.Matches(msg, key.NewBinding(key.WithKeys("up"))):
			if s.focusIndex <= 1 {
				// Move to input
				s.focusIndex = 0
				return s, s.updateFocus()
			}
			// On a VM selector, up moves to previous VM
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
	}

	return s, nil
}

// syncSelectorsFromInput updates selector options only when the input changed.
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
		// Preserve previous selection by string match
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
	s.nodesInput.SetWidth(width - 4)

	headerStyle := lipgloss.NewStyle().
		Foreground(tui.ColorCyan500).
		Bold(true)
	separator := lipgloss.NewStyle().
		Foreground(tui.ColorSlate700).
		Render(strings.Repeat("┄", width-4))

	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400).Width(20)
	activeStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)

	var b strings.Builder

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
	if focused {
		s.updateFocus()
	} else {
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
