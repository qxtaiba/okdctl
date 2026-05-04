package steps

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type placementPhase int

const (
	phaseDiscovering placementPhase = iota
	phasePlacing
)

// Field-key prefixes for per-node form fields. The constructor and the
// Apply function must agree on these — drift here silently breaks
// per-node placement read-back from the wizard step.
const (
	fieldPrefixMaster = "master"
	fieldPrefixWorker = "worker"
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

func (s *NodePlacementStep) IsWizardStepState() {}

// NewNodePlacementStep constructs the node placement wizard step.
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

// ShouldShow shows this step only when the Proxmox provider is selected.
func (s *NodePlacementStep) ShouldShow(cfg *config.Config) bool {
	if cfg.Provider.Type != config.ProviderProxmox {
		return false
	}
	s.cfg = cfg
	return true
}

// Init kicks off the Proxmox discovery fetch and spins the loading indicator.
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
			infraFields = append(infraFields,
				wizard.FieldDefinition{
					Key: fieldBridge, Label: fieldBridge, Default: firstMatch(bridges, px.Bridge, "vmbr0"),
					Help: "network bridge for vms", Type: wizard.FieldTypeSelect, Options: bridges,
					ConfigSet: func(cfg *config.Config, v string) error { cfg.Provider.Proxmox.Bridge = v; return nil },
					ConfigGet: func(cfg *config.Config) string { return cfg.Provider.Proxmox.Bridge },
				},
				additionalNetworksField(bridges, px.AdditionalNetworks),
			)
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
					Key: "data_storage", Label: fieldDataStorage, Default: firstMatch(pools, px.DataStorage, "local-lvm"),
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
		if len(disc.ISOs) > 0 {
			infraFields = append(infraFields, fcosISOField(disc.ISOs, px.FCOSIso))
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
		sections = append(sections, nodePlacementSection("control plane", fieldPrefixMaster, clusterName, cpCount, px.MasterNodes, defaultNode, nodeNames))
	}

	wCount := s.cfg.Topology.Workers.Count
	if wCount > 0 {
		sections = append(sections, nodePlacementSection("workers", fieldPrefixWorker, clusterName, wCount, px.WorkerNodes, defaultNode, nodeNames))
	}

	def := wizard.StepDefinition{
		ID:           wizard.StepIDNodePlacement,
		Title:        "proxmox infrastructure",
		DisplayTitle: "configure proxmox infrastructure",
		Description:  "auto-discovered from your proxmox cluster",
		Sections:     sections,
		Apply: func(step *wizard.DataDrivenStep, cfg *config.Config) error {
			var masterNodes, workerNodes []string
			for i := range cpCount {
				masterNodes = append(masterNodes, step.Value(fmt.Sprintf("%s_%d", fieldPrefixMaster, i)))
			}
			for i := range wCount {
				workerNodes = append(workerNodes, step.Value(fmt.Sprintf("%s_%d", fieldPrefixWorker, i)))
			}
			cfg.Provider.Proxmox.MasterNodes = masterNodes
			cfg.Provider.Proxmox.WorkerNodes = workerNodes
			return nil
		},
	}

	s.inner = wizard.NewDataDrivenStep(&def)
	s.inner.LoadFromConfig(s.cfg)
}

// Update handles discovery results, spinner ticks, and forwards other
// input to the built inner form once discovery completes.
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

// View renders either the loading spinner or the inner placement form.
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

// Apply delegates to the inner form's Apply when one has been built.
func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if s.inner != nil {
		return s.inner.Apply(cfg)
	}
	return nil
}

// SetFocused propagates focus to the inner form.
func (s *NodePlacementStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if s.inner != nil {
		s.inner.SetFocused(focused)
	}
}

// ShortHelp returns the step's help bar or nil while discovering.
func (s *NodePlacementStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == phaseDiscovering {
		return nil
	}
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: helpNavigate},
		{Key: "← →", Help: "change value"},
		{Key: helpEnter, Help: helpConfirm},
		{Key: helpEsc, Help: helpBack},
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

func additionalNetworksField(bridges []string, current []config.AdditionalNetwork) wizard.FieldDefinition {
	return wizard.FieldDefinition{
		Key:     "additional_networks",
		Label:   "additional networks",
		Default: additionalNetworksBridges(current),
		Help:    "extra bridges to attach to all vms — leave empty for none",
		Type:    wizard.FieldTypeMultiSelect,
		Options: bridges,
		ConfigSet: func(cfg *config.Config, v string) error {
			cfg.Provider.Proxmox.AdditionalNetworks = parseAdditionalNetworks(v, cfg.Provider.Proxmox.AdditionalNetworks)
			return nil
		},
		ConfigGet: func(cfg *config.Config) string {
			return additionalNetworksBridges(cfg.Provider.Proxmox.AdditionalNetworks)
		},
	}
}

func fcosISOField(isos []string, current string) wizard.FieldDefinition {
	return wizard.FieldDefinition{
		Key:     "fcos_iso",
		Label:   "fcos iso",
		Default: firstMatch(isos, current, ""),
		Help:    "pre-uploaded coreos iso — blank to let okdctl download and upload it",
		Type:    wizard.FieldTypeSelect,
		Options: append([]string{""}, isos...),
		ConfigSet: func(cfg *config.Config, v string) error {
			cfg.Provider.Proxmox.FCOSIso = v
			return nil
		},
		ConfigGet: func(cfg *config.Config) string { return cfg.Provider.Proxmox.FCOSIso },
	}
}

// additionalNetworksBridges serialises []AdditionalNetwork to a comma-
// separated list of bridge names for round-tripping through the wizard's
// string-valued field model.
func additionalNetworksBridges(nets []config.AdditionalNetwork) string {
	names := make([]string, len(nets))
	for i, n := range nets {
		names[i] = n.Bridge
	}
	return strings.Join(names, ",")
}

// parseAdditionalNetworks converts a comma-separated bridge-name string into
// []AdditionalNetwork. Pre-existing entries matched by Bridge are preserved
// intact (keeping hand-authored Model and VLANTag values). Newly selected
// bridges get Model "virtio". Empty input returns nil.
func parseAdditionalNetworks(v string, existing []config.AdditionalNetwork) []config.AdditionalNetwork {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	byBridge := make(map[string]config.AdditionalNetwork, len(existing))
	for _, n := range existing {
		byBridge[n.Bridge] = n
	}
	parts := strings.Split(v, ",")
	nets := make([]config.AdditionalNetwork, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if prev, ok := byBridge[p]; ok {
			nets = append(nets, prev)
		} else {
			nets = append(nets, config.AdditionalNetwork{Bridge: p, Model: "virtio"})
		}
	}
	return nets
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
