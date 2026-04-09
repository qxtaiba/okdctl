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

// vmAssignment pairs a VM name with a CompactSelector for choosing its Proxmox node.
type vmAssignment struct {
	name     string
	selector *components.CompactSelector
}

// NodePlacementStep lets users assign each VM to a specific Proxmox node.
// It dynamically generates per-VM selectors based on the topology from earlier steps.
type NodePlacementStep struct {
	wizard.BaseStep

	cfg *config.Config // captured from ShouldShow

	// "available nodes" text input
	nodesInput *components.InputField

	// per-VM selectors
	masters []vmAssignment
	workers []vmAssignment

	// navigation: 0 = nodesInput, 1..N = VM selectors
	focusIndex int
	totalSlots int // 1 (input) + len(masters) + len(workers)

	// cached state
	availableNodes []string
	built          bool
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

// ShouldShow is called by the wizard navigator with the current config.
// We use it to capture the config and rebuild selectors if topology changed.
func (s *NodePlacementStep) ShouldShow(cfg *config.Config) bool {
	if cfg.Provider.Type != config.ProviderProxmox {
		return false
	}
	s.cfg = cfg
	s.rebuildSelectors()
	return true
}

func (s *NodePlacementStep) Init() tea.Cmd {
	if s.cfg != nil {
		s.rebuildSelectors()
	}
	s.focusIndex = 0
	return s.updateFocus()
}

func (s *NodePlacementStep) rebuildSelectors() {
	if s.cfg == nil || s.cfg.Provider.Proxmox == nil {
		return
	}

	defaultNode := s.cfg.Provider.Proxmox.Node
	if defaultNode == "" {
		defaultNode = "pve"
	}

	// Parse available nodes from input, or use default
	nodes := parseNodeList(s.nodesInput.Value())
	if len(nodes) == 0 {
		nodes = []string{defaultNode}
		s.nodesInput.SetDefault(defaultNode)
	}
	s.availableNodes = nodes

	masterCount := s.cfg.Topology.ControlPlane.Count
	workerCount := s.cfg.Topology.Workers.Count

	// Rebuild master selectors
	s.masters = buildVMAssignments(s.cfg.Cluster.Name, "master", masterCount, nodes, s.cfg.Provider.Proxmox.MasterNodes, defaultNode)
	s.workers = buildVMAssignments(s.cfg.Cluster.Name, "worker", workerCount, nodes, s.cfg.Provider.Proxmox.WorkerNodes, defaultNode)

	s.totalSlots = 1 + len(s.masters) + len(s.workers)
	if s.focusIndex >= s.totalSlots {
		s.focusIndex = 0
	}
	s.built = true
}

func buildVMAssignments(clusterName, role string, count int, nodes, configured []string, defaultNode string) []vmAssignment {
	assignments := make([]vmAssignment, count)
	for i := range count {
		name := fmt.Sprintf("%s-%s%d", clusterName, role, i)
		sel := components.NewCompactSelector(nodes)
		sel.SetFocused(false)

		// Pre-select from config if available, otherwise default node
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
			// Reparse nodes input and rebuild selectors before completing
			s.rebuildSelectors()
			return s, func() tea.Msg {
				return wizard.StepCompleteMsg{StepID: s.ID()}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "down"))):
			if s.focusIndex == 0 {
				// Leaving the text input — rebuild selectors from its value
				s.nodesInput.Blur()
				s.rebuildSelectorsFromInput()
			}
			s.focusIndex++
			if s.focusIndex >= s.totalSlots {
				s.focusIndex = s.totalSlots - 1
			}
			return s, s.updateFocus()

		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab", "up"))):
			s.focusIndex--
			if s.focusIndex < 0 {
				s.focusIndex = 0
			}
			if s.focusIndex == 0 {
				s.rebuildSelectorsFromInput()
			}
			return s, s.updateFocus()

		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "right"))):
			// For VM selectors, left/right cycles through node options
			if s.focusIndex > 0 {
				sel := s.selectorAt(s.focusIndex)
				if sel != nil {
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
				return s, s.emitFocusChanged()
			}
		}

		// Pass to nodesInput if focused on it
		if s.focusIndex == 0 {
			var cmd tea.Cmd
			s.nodesInput, cmd = s.nodesInput.Update(msg)
			return s, cmd
		}
	}

	return s, nil
}

func (s *NodePlacementStep) rebuildSelectorsFromInput() {
	nodes := parseNodeList(s.nodesInput.Value())
	if len(nodes) == 0 {
		return
	}
	s.availableNodes = nodes

	// Update selector options while preserving selections
	updateAssignmentOptions(s.masters, nodes)
	updateAssignmentOptions(s.workers, nodes)
}

func updateAssignmentOptions(assignments []vmAssignment, nodes []string) {
	for i := range assignments {
		current := assignments[i].selector.Selected()
		assignments[i].selector.SetOptions(nodes)
		// Try to preserve previous selection
		for j, n := range nodes {
			if n == current {
				assignments[i].selector.SetSelected(j)
				break
			}
		}
	}
}

func (s *NodePlacementStep) selectorAt(index int) *components.CompactSelector {
	vmIndex := index - 1 // offset by 1 for nodesInput
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
	return s.emitFocusChanged()
}

func (s *NodePlacementStep) emitFocusChanged() tea.Cmd {
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
	separatorStyle := lipgloss.NewStyle().
		Foreground(tui.ColorSlate700)
	separator := separatorStyle.Render(strings.Repeat("┄", width-4))

	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400).Width(20)
	selectedStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	focusIndicator := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)

	var b strings.Builder

	// Available nodes input
	b.WriteString(headerStyle.Render("available proxmox nodes"))
	b.WriteString("\n")
	b.WriteString(separator)
	b.WriteString("\n")
	b.WriteString(s.nodesInput.View())
	b.WriteString("\n\n")

	// Control plane placement
	if len(s.masters) > 0 {
		b.WriteString(headerStyle.Render("control plane placement"))
		b.WriteString("\n")
		b.WriteString(separator)
		b.WriteString("\n")
		for i, m := range s.masters {
			globalIdx := 1 + i
			isFocused := s.focusIndex == globalIdx

			label := labelStyle.Render(m.name)
			node := m.selector.Selected()
			var nodeDisplay string
			if isFocused {
				nodeDisplay = focusIndicator.Render("▸ ") + selectedStyle.Render(node) + focusIndicator.Render(" ◂")
				if len(s.availableNodes) > 1 {
					nodeDisplay += unselectedStyle.Render("  ← →")
				}
			} else {
				nodeDisplay = selectedStyle.Render(node)
			}
			b.WriteString(label + nodeDisplay)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Worker placement
	if len(s.workers) > 0 {
		b.WriteString(headerStyle.Render("worker placement"))
		b.WriteString("\n")
		b.WriteString(separator)
		b.WriteString("\n")
		for i, w := range s.workers {
			globalIdx := 1 + len(s.masters) + i
			isFocused := s.focusIndex == globalIdx

			label := labelStyle.Render(w.name)
			node := w.selector.Selected()
			var nodeDisplay string
			if isFocused {
				nodeDisplay = focusIndicator.Render("▸ ") + selectedStyle.Render(node) + focusIndicator.Render(" ◂")
				if len(s.availableNodes) > 1 {
					nodeDisplay += unselectedStyle.Render("  ← →")
				}
			} else {
				nodeDisplay = selectedStyle.Render(node)
			}
			b.WriteString(label + nodeDisplay)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}

	// Rebuild selectors one final time from current input
	s.rebuildSelectorsFromInput()

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
