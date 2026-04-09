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

type discoveryCompleteMsg struct {
	discovery *proxmoxDiscovery
	err       error
}

type vmAssignment struct {
	name     string
	selector *components.CompactSelector
}

// namedSelector is a labeled CompactSelector for infrastructure fields.
type namedSelector struct {
	label    string
	selector *components.CompactSelector
}

type NodePlacementStep struct {
	wizard.BaseStep

	cfg   *config.Config
	phase placementPhase

	loadingSpinner spinner.Model
	discovery      *proxmoxDiscovery
	discoveryErr   error

	// Infrastructure selectors (bridge, storage)
	infra []namedSelector

	// VM node selectors
	bootstrap *components.CompactSelector
	masters   []vmAssignment
	workers   []vmAssignment

	// Flat focus list: infra[0..I-1], bootstrap=I, masters[I+1..], workers[...]
	focusIndex     int
	totalSlots     int
	infraCount     int // number of infra selectors
	availableNodes []string
}

func NewNodePlacementStep() (*NodePlacementStep, *NodePlacementStep) {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	s := &NodePlacementStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDNodePlacement,
			"proxmox infrastructure",
			"configure proxmox infrastructure",
			"auto-discovered from your proxmox cluster",
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
	return tea.Batch(s.loadingSpinner.Tick, s.fetchDiscovery)
}

func (s *NodePlacementStep) fetchDiscovery() tea.Msg {
	disc, err := discoverProxmox(s.cfg)
	return discoveryCompleteMsg{discovery: disc, err: err}
}

func (s *NodePlacementStep) build(disc *proxmoxDiscovery, nodeNames []string) {
	px := s.cfg.Provider.Proxmox
	s.availableNodes = nodeNames

	// --- Infrastructure selectors ---
	s.infra = nil

	// Bridge
	if disc != nil && len(disc.Bridges) > 0 {
		names := make([]string, len(disc.Bridges))
		for i, b := range disc.Bridges {
			names[i] = b.Name
		}
		s.infra = append(s.infra, makeNamedSelector("bridge", names, px.Bridge, "vmbr0"))
	}

	// OS storage — pools that support "images"
	if disc != nil {
		if pools := filterStorageByContent(disc.Storage, "images"); len(pools) > 0 {
			s.infra = append(s.infra, makeNamedSelector("os storage", pools, px.Storage, "local-lvm"))
		}
	}

	// Data storage — same pools
	if disc != nil {
		if pools := filterStorageByContent(disc.Storage, "images"); len(pools) > 0 {
			s.infra = append(s.infra, makeNamedSelector("data storage", pools, px.DataStorage, "local-lvm"))
		}
	}

	// ISO storage — pools that support "iso"
	if disc != nil {
		if pools := filterStorageByContent(disc.Storage, "iso"); len(pools) > 0 {
			s.infra = append(s.infra, makeNamedSelector("iso storage", pools, px.ISOStorage, "local"))
		}
	}

	s.infraCount = len(s.infra)

	// --- Node selectors ---
	defaultNode := nodeNames[0]
	if px.Node != "" {
		for _, n := range nodeNames {
			if n == px.Node {
				defaultNode = px.Node
				break
			}
		}
	}

	s.bootstrap = components.NewCompactSelector(nodeNames)
	s.bootstrap.SetFocused(false)
	selectByName(s.bootstrap, nodeNames, defaultNode)

	s.masters = buildVMAssignments(
		s.cfg.Cluster.Name, "master",
		s.cfg.Topology.ControlPlane.Count,
		nodeNames, px.MasterNodes, defaultNode,
	)
	s.workers = buildVMAssignments(
		s.cfg.Cluster.Name, "worker",
		s.cfg.Topology.Workers.Count,
		nodeNames, px.WorkerNodes, defaultNode,
	)

	s.totalSlots = s.infraCount + 1 + len(s.masters) + len(s.workers)
	s.focusIndex = 0
}

func filterStorageByContent(storage []proxmoxStorage, content string) []string {
	var names []string
	for _, st := range storage {
		if strings.Contains(st.Content, content) {
			names = append(names, st.Name)
		}
	}
	return names
}

func makeNamedSelector(label string, options []string, current, fallback string) namedSelector {
	sel := components.NewCompactSelector(options)
	sel.SetFocused(false)
	target := fallback
	if current != "" {
		target = current
	}
	selectByName(sel, options, target)
	return namedSelector{label: label, selector: sel}
}

func selectByName(sel *components.CompactSelector, options []string, target string) {
	for i, n := range options {
		if n == target {
			sel.SetSelected(i)
			return
		}
	}
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
		selectByName(sel, nodes, target)

		assignments[i] = vmAssignment{name: name, selector: sel}
	}
	return assignments
}

func (s *NodePlacementStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case discoveryCompleteMsg:
		s.discovery = msg.discovery
		s.discoveryErr = msg.err
		s.phase = phasePlacing

		var nodeNames []string
		if msg.err == nil && msg.discovery != nil && len(msg.discovery.Nodes) > 0 {
			nodeNames = make([]string, len(msg.discovery.Nodes))
			for i, n := range msg.discovery.Nodes {
				nodeNames[i] = n.Name
			}
		} else {
			fallback := s.cfg.Provider.Proxmox.Node
			if fallback == "" {
				fallback = "pve"
			}
			nodeNames = []string{fallback}
		}

		s.build(msg.discovery, nodeNames)
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
	// infra selectors: 0..infraCount-1
	if index < s.infraCount {
		return s.infra[index].selector
	}
	index -= s.infraCount

	// bootstrap: index 0 after infra offset
	if index == 0 {
		return s.bootstrap
	}
	index-- // skip bootstrap

	// masters
	if index < len(s.masters) {
		return s.masters[index].selector
	}
	index -= len(s.masters)

	// workers
	if index < len(s.workers) {
		return s.workers[index].selector
	}
	return nil
}

func (s *NodePlacementStep) updateFocus() tea.Cmd {
	for i := range s.infra {
		s.infra[i].selector.SetFocused(false)
	}
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
		return s.loadingSpinner.View() + " discovering proxmox infrastructure..."
	}

	headerStyle := lipgloss.NewStyle().Foreground(tui.ColorCyan500).Bold(true)
	separator := lipgloss.NewStyle().Foreground(tui.ColorSlate700).Render(strings.Repeat("┄", width-4))
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400).Width(20)
	activeStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	successStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)

	var b strings.Builder

	// Discovery banner
	if s.discoveryErr != nil {
		warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
		b.WriteString(warnStyle.Render("  " + s.discoveryErr.Error()))
		b.WriteString("\n\n")
	} else if s.discovery != nil {
		nodeParts := make([]string, len(s.discovery.Nodes))
		for i, n := range s.discovery.Nodes {
			info := n.Name
			if n.CPUs > 0 || n.MemGB > 0 {
				info += fmt.Sprintf(" (%d cpu, %d gb)", n.CPUs, n.MemGB)
			}
			if n.Status != "online" {
				info += " [" + n.Status + "]"
			}
			nodeParts[i] = info
		}
		b.WriteString(successStyle.Render("  discovered " + fmt.Sprintf("%d", len(s.discovery.Nodes)) + " node(s), " +
			fmt.Sprintf("%d", len(s.discovery.Storage)) + " storage pool(s), " +
			fmt.Sprintf("%d", len(s.discovery.Bridges)) + " bridge(s)"))
		b.WriteString("\n\n")
	}

	renderRow := func(label string, sel *components.CompactSelector, globalIdx int) {
		isFocused := s.focusIndex == globalIdx
		lbl := labelStyle.Render(label)
		val := sel.Selected()
		if isFocused {
			hint := ""
			if sel.Len() > 1 {
				hint = dimStyle.Render("  ← →")
			}
			b.WriteString(lbl + activeStyle.Render("▸ "+val+" ◂") + hint)
		} else {
			b.WriteString(lbl + dimStyle.Render(val))
		}
		b.WriteString("\n")
	}

	// Infrastructure section
	if s.infraCount > 0 {
		b.WriteString(headerStyle.Render("infrastructure"))
		b.WriteString("\n")
		b.WriteString(separator)
		b.WriteString("\n")
		for i, inf := range s.infra {
			renderRow(inf.label, inf.selector, i)
		}
		b.WriteString("\n")
	}

	// Bootstrap
	clusterName := s.cfg.Cluster.Name
	if clusterName == "" {
		clusterName = "cluster"
	}
	b.WriteString(headerStyle.Render("bootstrap"))
	b.WriteString("\n")
	b.WriteString(separator)
	b.WriteString("\n")
	renderRow(clusterName+"-bootstrap", s.bootstrap, s.infraCount)
	b.WriteString("\n")

	// Control plane
	if len(s.masters) > 0 {
		b.WriteString(headerStyle.Render("control plane"))
		b.WriteString("\n")
		b.WriteString(separator)
		b.WriteString("\n")
		for i, a := range s.masters {
			renderRow(a.name, a.selector, s.infraCount+1+i)
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
			renderRow(a.name, a.selector, s.infraCount+1+len(s.masters)+i)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}
	px := cfg.Provider.Proxmox

	// Infrastructure selectors → config
	for _, inf := range s.infra {
		val := inf.selector.Selected()
		switch inf.label {
		case "bridge":
			px.Bridge = val
		case "os storage":
			px.Storage = val
		case "data storage":
			px.DataStorage = val
		case "iso storage":
			px.ISOStorage = val
		}
	}

	// Bootstrap node
	if s.bootstrap != nil {
		px.Node = s.bootstrap.Selected()
	}

	// Master/worker nodes
	if len(s.masters) > 0 {
		masterNodes := make([]string, len(s.masters))
		for i, m := range s.masters {
			masterNodes[i] = m.selector.Selected()
		}
		px.MasterNodes = masterNodes
	}
	if len(s.workers) > 0 {
		workerNodes := make([]string, len(s.workers))
		for i, w := range s.workers {
			workerNodes[i] = w.selector.Selected()
		}
		px.WorkerNodes = workerNodes
	}

	return nil
}

func (s *NodePlacementStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused && s.phase == phasePlacing {
		s.updateFocus()
	} else if !focused {
		for i := range s.infra {
			s.infra[i].selector.SetFocused(false)
		}
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
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "select field"},
		{Key: "← →", Help: "change value"},
		{Key: "enter", Help: "confirm"},
		{Key: "esc", Help: "back"},
	}
}
