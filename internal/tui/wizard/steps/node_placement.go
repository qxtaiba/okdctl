package steps

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
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

// NodePlacementStep discovers Proxmox infrastructure and presents
// selectable dropdowns for bridge, storage, and per-VM node assignment.
// It uses a two-phase approach: spinner during discovery, then a
// dynamically-built DataDrivenStep for the selection UI.
type NodePlacementStep struct {
	wizard.BaseStep

	cfg   *config.Config
	phase placementPhase

	loadingSpinner spinner.Model
	discovery      *proxmoxDiscovery
	discoveryErr   error

	// The inner form step, built dynamically after discovery
	inner *wizard.DataDrivenStep
}

func NewNodePlacementStep() (step, state *NodePlacementStep) {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	step = &NodePlacementStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDNodePlacement,
			"proxmox infrastructure",
			"configure proxmox infrastructure",
			"auto-discovered from your proxmox cluster",
		),
		loadingSpinner: sp,
		phase:          phaseDiscovering,
	}
	return step, step
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

// buildInnerStep creates a DataDrivenStep with SelectField dropdowns
// populated from discovery results.
func (s *NodePlacementStep) buildInnerStep(disc *proxmoxDiscovery, nodeNames []string) {
	px := s.cfg.Provider.Proxmox
	clusterName := s.cfg.Cluster.Name
	if clusterName == "" {
		clusterName = "cluster"
	}

	var sections []wizard.SectionDefinition

	// Infrastructure section
	if disc != nil {
		var infraFields []wizard.FieldDefinition

		if bridges := bridgeNames(disc.Bridges); len(bridges) > 0 {
			infraFields = append(infraFields, wizard.FieldDefinition{
				Key: "bridge", Label: "bridge", Default: firstMatch(bridges, px.Bridge, "vmbr0"),
				Help: "network bridge for vms", Type: wizard.FieldTypeSelect, Options: bridges,
				ConfigSet: func(cfg *config.Config, v string) error { cfg.Provider.Proxmox.Bridge = v; return nil },
				ConfigGet: func(cfg *config.Config) string { return cfg.Provider.Proxmox.Bridge },
			})
		}
		if pools := filterStorageByContent(disc.Storage, "images"); len(pools) > 0 {
			infraFields = append(infraFields,
				wizard.FieldDefinition{
					Key: "os_storage", Label: "os storage", Default: firstMatch(pools, px.Storage, "local-lvm"),
					Help: "storage pool for vm boot disks", Type: wizard.FieldTypeSelect, Options: pools,
					ConfigSet: func(cfg *config.Config, v string) error { cfg.Provider.Proxmox.Storage = v; return nil },
					ConfigGet: func(cfg *config.Config) string { return cfg.Provider.Proxmox.Storage },
				},
				wizard.FieldDefinition{
					Key: "data_storage", Label: "data storage", Default: firstMatch(pools, px.DataStorage, "local-lvm"),
					Help: "storage pool for data/ceph disks", Type: wizard.FieldTypeSelect, Options: pools,
					ConfigSet: func(cfg *config.Config, v string) error { cfg.Provider.Proxmox.DataStorage = v; return nil },
					ConfigGet: func(cfg *config.Config) string { return cfg.Provider.Proxmox.DataStorage },
				},
			)
		}
		if pools := filterStorageByContent(disc.Storage, "iso"); len(pools) > 0 {
			infraFields = append(infraFields, wizard.FieldDefinition{
				Key: "iso_storage", Label: "iso storage", Default: firstMatch(pools, px.ISOStorage, "local"),
				Help: "storage for iso files", Type: wizard.FieldTypeSelect, Options: pools,
				ConfigSet: func(cfg *config.Config, v string) error { cfg.Provider.Proxmox.ISOStorage = v; return nil },
				ConfigGet: func(cfg *config.Config) string { return cfg.Provider.Proxmox.ISOStorage },
			})
		}

		if len(infraFields) > 0 {
			sections = append(sections, wizard.SectionDefinition{Title: "infrastructure", Fields: infraFields})
		}
	}

	// Node placement sections
	defaultNode := nodeNames[0]

	// Bootstrap
	sections = append(sections, wizard.SectionDefinition{
		Title: "bootstrap",
		Fields: []wizard.FieldDefinition{{
			Key: "bootstrap_node", Label: clusterName + "-bootstrap", Default: defaultNode,
			Help: "proxmox node for bootstrap vm", Type: wizard.FieldTypeSelect, Options: nodeNames,
			ConfigSet: func(cfg *config.Config, v string) error { cfg.Provider.Proxmox.Node = v; return nil },
			ConfigGet: func(cfg *config.Config) string { return cfg.Provider.Proxmox.Node },
		}},
	})

	cpCount := s.cfg.Topology.ControlPlane.Count
	if cpCount > 0 {
		sections = append(sections, nodePlacementSection("control plane", "master", clusterName, cpCount, px.MasterNodes, defaultNode, nodeNames))
	}

	wCount := s.cfg.Topology.Workers.Count
	if wCount > 0 {
		sections = append(sections, nodePlacementSection("workers", "worker", clusterName, wCount, px.WorkerNodes, defaultNode, nodeNames))
	}

	def := wizard.StepDefinition{
		ID:           wizard.StepIDNodePlacement,
		Title:        "proxmox infrastructure",
		DisplayTitle: "configure proxmox infrastructure",
		Description:  "auto-discovered from your proxmox cluster",
		Sections:     sections,
		Apply: func(step *wizard.DataDrivenStep, cfg *config.Config) error {
			// Master/worker nodes from individual fields
			var masterNodes, workerNodes []string
			for i := range cpCount {
				masterNodes = append(masterNodes, step.Value(fmt.Sprintf("master_%d", i)))
			}
			for i := range wCount {
				workerNodes = append(workerNodes, step.Value(fmt.Sprintf("worker_%d", i)))
			}
			cfg.Provider.Proxmox.MasterNodes = masterNodes
			cfg.Provider.Proxmox.WorkerNodes = workerNodes
			return nil
		},
	}

	s.inner = wizard.NewDataDrivenStep(&def)
	s.inner.LoadFromConfig(s.cfg)
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

		s.buildInnerStep(s.discovery, nodeNames)
		return s, s.inner.Init()

	case spinner.TickMsg:
		if s.phase == phaseDiscovering {
			var cmd tea.Cmd
			s.loadingSpinner, cmd = s.loadingSpinner.Update(msg)
			return s, cmd
		}
	}

	// Delegate to inner step once built
	if s.phase == phasePlacing && s.inner != nil {
		_, cmd := s.inner.Update(msg)
		return s, cmd
	}

	return s, nil
}

func (s *NodePlacementStep) View(width, height int) string {
	s.SetSize(width, height)

	if s.phase == phaseDiscovering {
		return s.loadingSpinner.View() + " discovering proxmox infrastructure..."
	}

	noteStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500).Italic(true).PaddingLeft(2)
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning).PaddingLeft(2)

	var header string
	if s.discoveryErr != nil {
		header = warnStyle.Render(s.discoveryErr.Error()) + "\n\n"
	} else if s.discovery != nil {
		header = noteStyle.Render(fmt.Sprintf("discovered %d node(s), %d storage pool(s), %d bridge(s)",
			len(s.discovery.Nodes), len(s.discovery.Storage), len(s.discovery.Bridges))) + "\n\n"
	}

	if s.inner != nil {
		return header + s.inner.View(width, height)
	}
	return header
}

func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if s.inner != nil {
		return s.inner.Apply(cfg)
	}
	return nil
}

func (s *NodePlacementStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if s.inner != nil {
		s.inner.SetFocused(focused)
	}
}

func (s *NodePlacementStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == phaseDiscovering {
		return nil
	}
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "navigate"},
		{Key: "← →", Help: "change value"},
		{Key: "enter", Help: "confirm"},
		{Key: "esc", Help: "back"},
	}
}

// nodePlacementSection builds a SectionDefinition of proxmox-node select
// fields for a role (bootstrap, master, worker). fieldPrefix is used for
// both the field key (e.g. "master_0") and the label suffix.
func nodePlacementSection(title, fieldPrefix, clusterName string, count int, existing []string, defaultNode string, allNodes []string) wizard.SectionDefinition {
	fields := make([]wizard.FieldDefinition, 0, count)
	for i := range count {
		target := defaultNode
		if i < len(existing) && existing[i] != "" {
			target = existing[i]
		}
		fields = append(fields, wizard.FieldDefinition{
			Key:     fmt.Sprintf("%s_%d", fieldPrefix, i),
			Label:   fmt.Sprintf("%s-%s%d", clusterName, fieldPrefix, i),
			Default: target, Help: "proxmox node",
			Type: wizard.FieldTypeSelect, Options: allNodes,
		})
	}
	return wizard.SectionDefinition{Title: title, Fields: fields}
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

func firstMatch(options []string, current, fallback string) string {
	if current != "" {
		for _, o := range options {
			if o == current {
				return current
			}
		}
	}
	for _, o := range options {
		if o == fallback {
			return fallback
		}
	}
	if len(options) > 0 {
		return options[0]
	}
	return ""
}
