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

type namedSelector struct {
	label    string
	selector *components.CompactSelector
}

// placementSection groups selectors under a header for rendering.
type placementSection struct {
	title string
	rows  []placementRow
}

type placementRow struct {
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

	// All selectors organized into sections for rendering
	sections []placementSection

	// Flat index into all rows across all sections
	focusIndex int
	totalSlots int
	allRows    []placementRow // flattened for indexed access

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
	s.sections = nil

	defaultNode := nodeNames[0]
	if px.Node != "" {
		for _, n := range nodeNames {
			if n == px.Node {
				defaultNode = px.Node
				break
			}
		}
	}

	// Infrastructure section
	var infraRows []placementRow
	if disc != nil {
		if bridges := bridgeNames(disc.Bridges); len(bridges) > 0 {
			infraRows = append(infraRows, makePlacementRow("bridge", bridges, px.Bridge, "vmbr0"))
		}
		if pools := filterStorageByContent(disc.Storage, "images"); len(pools) > 0 {
			infraRows = append(infraRows, makePlacementRow("os storage", pools, px.Storage, "local-lvm"))
			infraRows = append(infraRows, makePlacementRow("data storage", pools, px.DataStorage, "local-lvm"))
		}
		if pools := filterStorageByContent(disc.Storage, "iso"); len(pools) > 0 {
			infraRows = append(infraRows, makePlacementRow("iso storage", pools, px.ISOStorage, "local"))
		}
	}
	if len(infraRows) > 0 {
		s.sections = append(s.sections, placementSection{title: "infrastructure", rows: infraRows})
	}

	// Bootstrap section
	bootstrapSel := components.NewCompactSelector(nodeNames)
	bootstrapSel.SetFocused(false)
	selectByName(bootstrapSel, nodeNames, defaultNode)
	clusterName := s.cfg.Cluster.Name
	if clusterName == "" {
		clusterName = "cluster"
	}
	s.sections = append(s.sections, placementSection{
		title: "bootstrap",
		rows:  []placementRow{{label: clusterName + "-bootstrap", selector: bootstrapSel}},
	})

	// Control plane section
	if s.cfg.Topology.ControlPlane.Count > 0 {
		var masterRows []placementRow
		for i := range s.cfg.Topology.ControlPlane.Count {
			name := fmt.Sprintf("%s-master%d", clusterName, i)
			sel := components.NewCompactSelector(nodeNames)
			sel.SetFocused(false)
			target := defaultNode
			if i < len(px.MasterNodes) && px.MasterNodes[i] != "" {
				target = px.MasterNodes[i]
			}
			selectByName(sel, nodeNames, target)
			masterRows = append(masterRows, placementRow{label: name, selector: sel})
		}
		s.sections = append(s.sections, placementSection{title: "control plane", rows: masterRows})
	}

	// Workers section
	if s.cfg.Topology.Workers.Count > 0 {
		var workerRows []placementRow
		for i := range s.cfg.Topology.Workers.Count {
			name := fmt.Sprintf("%s-worker%d", clusterName, i)
			sel := components.NewCompactSelector(nodeNames)
			sel.SetFocused(false)
			target := defaultNode
			if i < len(px.WorkerNodes) && px.WorkerNodes[i] != "" {
				target = px.WorkerNodes[i]
			}
			selectByName(sel, nodeNames, target)
			workerRows = append(workerRows, placementRow{label: name, selector: sel})
		}
		s.sections = append(s.sections, placementSection{title: "workers", rows: workerRows})
	}

	// Build flat index
	s.allRows = nil
	for _, sec := range s.sections {
		s.allRows = append(s.allRows, sec.rows...)
	}
	s.totalSlots = len(s.allRows)
	s.focusIndex = 0
}

func bridgeNames(bridges []proxmoxBridge) []string {
	names := make([]string, len(bridges))
	for i, b := range bridges {
		names[i] = b.Name
	}
	return names
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

func makePlacementRow(label string, options []string, current, fallback string) placementRow {
	sel := components.NewCompactSelector(options)
	sel.SetFocused(false)
	target := fallback
	if current != "" {
		target = current
	}
	selectByName(sel, options, target)
	return placementRow{label: label, selector: sel}
}

func selectByName(sel *components.CompactSelector, options []string, target string) {
	for i, n := range options {
		if n == target {
			sel.SetSelected(i)
			return
		}
	}
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
		if s.focusIndex < len(s.allRows) {
			sel := s.allRows[s.focusIndex].selector
			if sel.Len() > 1 {
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
		}
		return s, nil
	}

	return s, nil
}

func (s *NodePlacementStep) updateFocus() tea.Cmd {
	for i := range s.allRows {
		s.allRows[i].selector.SetFocused(i == s.focusIndex)
	}
	return func() tea.Msg {
		return wizard.FocusChangedMsg{
			FieldIndex:  s.focusIndex,
			TotalFields: s.totalSlots,
		}
	}
}

// --- View ---

type placementStyles struct {
	sectionActive  lipgloss.Style
	sectionPending lipgloss.Style
	sectionDone    lipgloss.Style
	label          lipgloss.Style
	valueActive    lipgloss.Style
	valueDim       lipgloss.Style
	hint           lipgloss.Style
	note           lipgloss.Style
	warn           lipgloss.Style
	sectionPad     lipgloss.Style
}

func newPlacementStyles() placementStyles {
	return placementStyles{
		sectionActive: lipgloss.NewStyle().
			Foreground(tui.ColorCyan500).Bold(true),
		sectionPending: lipgloss.NewStyle().
			Foreground(tui.ColorSlate600),
		sectionDone: lipgloss.NewStyle().
			Foreground(tui.ColorSuccess).Bold(true),
		label: lipgloss.NewStyle().
			Foreground(tui.ColorSlate300).Width(22),
		valueActive: lipgloss.NewStyle().
			Foreground(tui.ColorPrimary).Bold(true),
		valueDim: lipgloss.NewStyle().
			Foreground(tui.ColorSlate400),
		hint: lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).Italic(true),
		note: lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).Italic(true).PaddingLeft(2),
		warn: lipgloss.NewStyle().
			Foreground(tui.ColorWarning),
		sectionPad: lipgloss.NewStyle().Padding(0, 2),
	}
}

func (s *NodePlacementStep) View(width, height int) string {
	s.SetSize(width, height)

	if s.phase == phaseDiscovering {
		return s.loadingSpinner.View() + " discovering proxmox infrastructure..."
	}

	st := newPlacementStyles()
	var b strings.Builder

	// Discovery banner
	if s.discoveryErr != nil {
		b.WriteString(st.warn.Render("  "+s.discoveryErr.Error()) + "\n\n")
	} else if s.discovery != nil {
		b.WriteString(st.note.Render(fmt.Sprintf("discovered %d node(s), %d storage pool(s), %d bridge(s)",
			len(s.discovery.Nodes), len(s.discovery.Storage), len(s.discovery.Bridges))))
		b.WriteString("\n\n")
	}

	globalIdx := 0
	for secIdx, sec := range s.sections {
		// Section indicator + title
		sectionHasFocus := false
		for _, row := range sec.rows {
			if globalIdx <= s.focusIndex && s.focusIndex < globalIdx+len(sec.rows) {
				sectionHasFocus = true
				break
			}
			_ = row
		}

		var indicator string
		var titleStyle lipgloss.Style
		if sectionHasFocus {
			indicator = st.sectionActive.Render("●")
			titleStyle = st.sectionActive
		} else if s.focusIndex > globalIdx+len(sec.rows)-1 {
			indicator = st.sectionDone.Render("✓")
			titleStyle = st.sectionDone
		} else {
			indicator = st.sectionPending.Render("○")
			titleStyle = st.sectionPending
		}

		b.WriteString(indicator + " " + titleStyle.Render(sec.title))
		b.WriteString("\n\n")

		// Section content
		if sectionHasFocus || secIdx < len(s.sections) {
			var content strings.Builder
			for _, row := range sec.rows {
				isFocused := globalIdx == s.focusIndex
				lbl := st.label.Render(row.label)

				if isFocused {
					val := row.selector.Selected()
					content.WriteString(lbl + st.valueActive.Render("▸ "+val+" ◂"))
					if row.selector.Len() > 1 {
						content.WriteString(st.hint.Render("  ← →"))
					}
				} else {
					content.WriteString(lbl + st.valueDim.Render(row.selector.Selected()))
				}
				content.WriteString("\n")
				globalIdx++
			}
			b.WriteString(st.sectionPad.Render(content.String()))
		} else {
			globalIdx += len(sec.rows)
		}

		b.WriteString("\n")
	}

	return b.String()
}

// --- Apply ---

func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if cfg.Provider.Proxmox == nil {
		return nil
	}
	px := cfg.Provider.Proxmox

	// Walk sections and apply values
	masterIdx := 0
	workerIdx := 0
	var masterNodes, workerNodes []string

	for _, sec := range s.sections {
		for _, row := range sec.rows {
			val := row.selector.Selected()
			switch {
			case row.label == "bridge":
				px.Bridge = val
			case row.label == "os storage":
				px.Storage = val
			case row.label == "data storage":
				px.DataStorage = val
			case row.label == "iso storage":
				px.ISOStorage = val
			case strings.HasSuffix(row.label, "-bootstrap"):
				px.Node = val
			case strings.Contains(row.label, "-master"):
				masterNodes = append(masterNodes, val)
				masterIdx++
			case strings.Contains(row.label, "-worker"):
				workerNodes = append(workerNodes, val)
				workerIdx++
			}
		}
	}

	if len(masterNodes) > 0 {
		px.MasterNodes = masterNodes
	}
	if len(workerNodes) > 0 {
		px.WorkerNodes = workerNodes
	}

	return nil
}

// --- Focus & Help ---

func (s *NodePlacementStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused && s.phase == phasePlacing && len(s.allRows) > 0 {
		s.updateFocus()
	} else if !focused {
		for i := range s.allRows {
			s.allRows[i].selector.SetFocused(false)
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
