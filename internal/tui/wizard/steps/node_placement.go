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

	// per-VM selectors
	bootstrap *components.CompactSelector
	masters   []vmAssignment
	workers   []vmAssignment

	// navigation: 0 = bootstrap, 1..N = masters, N+1..M = workers
	focusIndex     int
	totalSlots     int
	availableNodes []string
}

func NewNodePlacementStep() (*NodePlacementStep, *NodePlacementStep) {
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
		loadingSpinner: sp,
		phase:          phaseDiscovering,
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

func (s *NodePlacementStep) buildSelectors(nodeNames []string) {
	defaultNode := nodeNames[0]
	// Use existing config node if it's in the discovered list
	if existing := s.cfg.Provider.Proxmox.Node; existing != "" {
		for _, n := range nodeNames {
			if n == existing {
				defaultNode = existing
				break
			}
		}
	}

	s.availableNodes = nodeNames

	// Bootstrap selector
	s.bootstrap = components.NewCompactSelector(nodeNames)
	s.bootstrap.SetFocused(false)
	for i, n := range nodeNames {
		if n == defaultNode {
			s.bootstrap.SetSelected(i)
			break
		}
	}

	s.masters = buildVMAssignments(
		s.cfg.Cluster.Name, "master",
		s.cfg.Topology.ControlPlane.Count,
		nodeNames, s.cfg.Provider.Proxmox.MasterNodes, defaultNode,
	)
	s.workers = buildVMAssignments(
		s.cfg.Cluster.Name, "worker",
		s.cfg.Topology.Workers.Count,
		nodeNames, s.cfg.Provider.Proxmox.WorkerNodes, defaultNode,
	)

	s.totalSlots = 1 + len(s.masters) + len(s.workers) // 1 for bootstrap
	s.focusIndex = 0
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
			nodeNames = make([]string, len(msg.nodes))
			for i, n := range msg.nodes {
				nodeNames[i] = n.Name
			}
		} else {
			// Discovery failed — use existing config node as the only option
			fallback := s.cfg.Provider.Proxmox.Node
			if fallback == "" {
				fallback = "pve"
			}
			nodeNames = []string{fallback}
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

	case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "down"))):
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
		return s, s.updateFocus()

	case key.Matches(msg, key.NewBinding(key.WithKeys("left", "right"))):
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

	return s, nil
}

func (s *NodePlacementStep) selectorAt(index int) *components.CompactSelector {
	if index == 0 {
		return s.bootstrap
	}
	vmIdx := index - 1 // offset for bootstrap
	if vmIdx < len(s.masters) {
		return s.masters[vmIdx].selector
	}
	vmIdx -= len(s.masters)
	if vmIdx < len(s.workers) {
		return s.workers[vmIdx].selector
	}
	return nil
}

func (s *NodePlacementStep) updateFocus() tea.Cmd {
	if s.bootstrap != nil {
		s.bootstrap.SetFocused(false)
	}
	for i := range s.masters {
		s.masters[i].selector.SetFocused(false)
	}
	for i := range s.workers {
		s.workers[i].selector.SetFocused(false)
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

	if s.phase == phaseDiscovering {
		return s.loadingSpinner.View() + " discovering proxmox nodes..."
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(tui.ColorCyan500).
		Bold(true)
	separator := lipgloss.NewStyle().
		Foreground(tui.ColorSlate700).
		Render(strings.Repeat("┄", width-4))
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400).Width(20)
	activeStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	successStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)

	var b strings.Builder

	// Discovery banner with node info
	if len(s.discovered) > 0 {
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
		b.WriteString(successStyle.Render("  discovered: " + strings.Join(parts, ", ")))
		b.WriteString("\n\n")
	}

	hint := ""
	if len(s.availableNodes) > 1 {
		hint = "  ← →"
	}

	renderVM := func(label string, sel *components.CompactSelector, globalIdx int) {
		isFocused := s.focusIndex == globalIdx
		lbl := labelStyle.Render(label)
		node := sel.Selected()
		if isFocused {
			b.WriteString(lbl + activeStyle.Render("▸ "+node+" ◂") + dimStyle.Render(hint))
		} else {
			b.WriteString(lbl + dimStyle.Render(node))
		}
		b.WriteString("\n")
	}

	// Bootstrap
	b.WriteString(headerStyle.Render("bootstrap"))
	b.WriteString("\n")
	b.WriteString(separator)
	b.WriteString("\n")
	clusterName := s.cfg.Cluster.Name
	if clusterName == "" {
		clusterName = "cluster"
	}
	renderVM(clusterName+"-bootstrap", s.bootstrap, 0)
	b.WriteString("\n")

	// Control plane
	if len(s.masters) > 0 {
		b.WriteString(headerStyle.Render("control plane"))
		b.WriteString("\n")
		b.WriteString(separator)
		b.WriteString("\n")
		for i, a := range s.masters {
			renderVM(a.name, a.selector, 1+i)
		}
		b.WriteString("\n")
	}

	// Workers
	if len(s.workers) > 0 {
		b.WriteString(headerStyle.Render("workers"))
		b.WriteString("\n")
		b.WriteString(separator)
		b.WriteString("\n")
		for i, a := range s.workers {
			renderVM(a.name, a.selector, 1+len(s.masters)+i)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}

	// Bootstrap node = the Node field (used as target_node in TF + bootstrap fallback)
	if s.bootstrap != nil {
		cfg.Provider.Proxmox.Node = s.bootstrap.Selected()
	}

	if len(s.masters) > 0 {
		masterNodes := make([]string, len(s.masters))
		for i, m := range s.masters {
			masterNodes[i] = m.selector.Selected()
		}
		cfg.Provider.Proxmox.MasterNodes = masterNodes
	}

	if len(s.workers) > 0 {
		workerNodes := make([]string, len(s.workers))
		for i, w := range s.workers {
			workerNodes[i] = w.selector.Selected()
		}
		cfg.Provider.Proxmox.WorkerNodes = workerNodes
	}

	return nil
}

func (s *NodePlacementStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused && s.phase == phasePlacing {
		s.updateFocus()
	} else if !focused {
		if s.bootstrap != nil {
			s.bootstrap.SetFocused(false)
		}
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
	bindings := []wizard.KeyBinding{
		{Key: "↑↓", Help: "select vm"},
	}
	if len(s.availableNodes) > 1 {
		bindings = append(bindings, wizard.KeyBinding{Key: "← →", Help: "change node"})
	}
	bindings = append(bindings,
		wizard.KeyBinding{Key: "enter", Help: "confirm"},
		wizard.KeyBinding{Key: "esc", Help: "back"},
	)
	return bindings
}
