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
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

type placementPhase int

const (
	phaseDiscovering placementPhase = iota
	phasePlacing
)

// Label suffixes for per-node form fields (e.g. "cluster-master0").
const (
	fieldPrefixMaster = "master"
	fieldPrefixWorker = "worker"
)

// Role display labels shown in wizard section titles and review output.
const (
	roleLabelControlPlane = "control plane"
	roleLabelWorkers      = "workers"
)

type discoveryCompleteMsg struct {
	discovery *proxmoxDiscovery
	err       error
}

// NodePlacementStep discovers Proxmox infrastructure and presents
// selectable dropdowns for bridge, storage, and per-VM node assignment.
type NodePlacementStep struct {
	wizard.BaseStep

	cfg   *config.Config
	phase placementPhase

	loadingSpinner spinner.Model
	discovery      *proxmoxDiscovery
	discoveryErr   error

	// inner is the post-discovery form; fields below alias into it, nil if
	// discovery didn't surface that field.
	inner *wizard.MultiSectionForm

	bridgeField        *components.SelectField
	additionalNetworks *components.MultiSelectField
	osStorageField     *components.SelectField
	dataStorageField   *components.SelectField
	isoStorageField    *components.SelectField
	fcosField          *components.SelectField
	bootstrapField     *components.SelectField
	controlPlaneFields []*components.SelectField
	workerFields       []*components.SelectField
}

// NewNodePlacementStep constructs the node placement wizard step.
func NewNodePlacementStep() *NodePlacementStep {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	return &NodePlacementStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDNodePlacement,
			"proxmox infrastructure",
			"configure proxmox infrastructure",
			"auto-discovered from your proxmox cluster",
		),
		loadingSpinner: sp,
		phase:          phaseDiscovering,
	}
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

// buildInnerStep builds the form's dropdowns, retaining typed field pointers so
// Apply can read them back directly.
func (s *NodePlacementStep) buildInnerStep(disc *proxmoxDiscovery, nodeNames []string) {
	px := s.cfg.Provider.Proxmox
	clusterName := s.cfg.Cluster.Name
	if clusterName == "" {
		clusterName = "cluster"
	}

	var sections []wizard.FormSection

	if disc != nil {
		var infraFields []components.FormField

		if bridges := bridgeNames(disc.Bridges); len(bridges) > 0 {
			s.bridgeField = newSelectField(fieldBridge, "network bridge for vms",
				bridges, firstMatch(bridges, px.Bridge, "vmbr0"), px.Bridge)
			s.additionalNetworks = components.NewMultiSelectField("additional networks", bridges)
			s.additionalNetworks.Help = "extra bridges to attach to all vms — leave empty for none"
			s.additionalNetworks.SetValue(additionalNetworksBridges(px.AdditionalNetworks))
			infraFields = append(infraFields, s.bridgeField, s.additionalNetworks)
		}
		if pools := filterStorageByContent(disc.Storage, "images"); len(pools) > 0 {
			s.osStorageField = newSelectField("os storage", "storage pool for vm boot disks",
				pools, firstMatch(pools, px.Storage, "local-lvm"), px.Storage)
			s.dataStorageField = newSelectField(fieldDataStorage, "storage pool for data/ceph disks",
				pools, firstMatch(pools, px.DataStorage, "local-lvm"), px.DataStorage)
			infraFields = append(infraFields, s.osStorageField, s.dataStorageField)
		}
		if pools := filterStorageByContent(disc.Storage, "iso"); len(pools) > 0 {
			s.isoStorageField = newSelectField("iso storage", "storage for iso files",
				pools, firstMatch(pools, px.ISOStorage, "local"), px.ISOStorage)
			infraFields = append(infraFields, s.isoStorageField)
		}
		if len(disc.ISOs) > 0 {
			isoOptions := append([]string{""}, disc.ISOs...)
			s.fcosField = newSelectField("fcos iso",
				"pre-uploaded coreos iso — blank to let okdctl download and upload it",
				isoOptions, firstMatch(disc.ISOs, px.FCOSIso, ""), px.FCOSIso)
			infraFields = append(infraFields, s.fcosField)
		}

		if len(infraFields) > 0 {
			sections = append(sections, wizard.FormSection{
				Title: "infrastructure",
				Group: components.NewInputGroup(infraFields...),
			})
		}
	}

	defaultNode := nodeNames[0]

	s.bootstrapField = newSelectField(clusterName+"-bootstrap", "proxmox node for bootstrap vm",
		nodeNames, defaultNode, px.Node)
	sections = append(sections, wizard.FormSection{
		Title: "bootstrap",
		Group: components.NewInputGroup(s.bootstrapField),
	})

	if cpCount := s.cfg.Topology.ControlPlane.Count; cpCount > 0 {
		s.controlPlaneFields = nodeSelectFields(fieldPrefixMaster, clusterName, cpCount, px.ControlPlaneNodes, defaultNode, nodeNames)
		sections = append(sections, wizard.FormSection{
			Title: roleLabelControlPlane,
			Group: selectFieldGroup(s.controlPlaneFields),
		})
	}

	if wCount := s.cfg.Topology.Workers.Count; wCount > 0 {
		s.workerFields = nodeSelectFields(fieldPrefixWorker, clusterName, wCount, px.WorkerNodes, defaultNode, nodeNames)
		sections = append(sections, wizard.FormSection{
			Title: roleLabelWorkers,
			Group: selectFieldGroup(s.workerFields),
		})
	}

	s.inner = wizard.NewMultiSectionForm(sections)
}

// newSelectField sets def then overlays current; SetValue("") is a no-op unless
// "" is itself an option (the fcos blank case).
func newSelectField(label, help string, options []string, def, current string) *components.SelectField {
	sf := components.NewSelectField(label, options)
	sf.Help = help
	sf.SetDefault(def)
	sf.SetValue(current)
	return sf
}

// nodeSelectFields builds per-node dropdowns seeded from existing assignments;
// no config-load overlay, so the default alone sets the value.
func nodeSelectFields(fieldPrefix, clusterName string, count int, existing []string, defaultNode string, allNodes []string) []*components.SelectField {
	fields := make([]*components.SelectField, 0, count)
	for i := range count {
		target := defaultNode
		if i < len(existing) && existing[i] != "" {
			target = existing[i]
		}
		sf := components.NewSelectField(fmt.Sprintf("%s-%s%d", clusterName, fieldPrefix, i), allNodes)
		sf.Help = "proxmox node"
		sf.SetDefault(target)
		fields = append(fields, sf)
	}
	return fields
}

func selectFieldGroup(fields []*components.SelectField) *components.InputGroup {
	ff := make([]components.FormField, len(fields))
	for i, f := range fields {
		ff[i] = f
	}
	return components.NewInputGroup(ff...)
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

	if s.phase == phasePlacing && s.inner != nil {
		cmd, enterPressed := s.inner.Update(msg)
		if !enterPressed {
			return s, cmd
		}
		if err := s.inner.Validate(); err != nil {
			return s, nil
		}
		return s, func() tea.Msg {
			return wizard.StepCompleteMsg{StepID: s.ID()}
		}
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
		return header + s.inner.View(width)
	}
	return header
}

// Apply writes each retained field's value into cfg; controlPlaneFields[i]/
// workerFields[i] map to ControlPlaneNodes[i]/WorkerNodes[i] by index, which
// is the correctness contract for VM-to-node assignment.
func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if s.inner == nil || cfg.Provider.Proxmox == nil {
		return nil
	}
	px := cfg.Provider.Proxmox

	if s.bridgeField != nil {
		px.Bridge = s.bridgeField.Value()
	}
	if s.additionalNetworks != nil {
		px.AdditionalNetworks = parseAdditionalNetworks(s.additionalNetworks.Value(), px.AdditionalNetworks)
	}
	if s.osStorageField != nil {
		px.Storage = s.osStorageField.Value()
	}
	if s.dataStorageField != nil {
		px.DataStorage = s.dataStorageField.Value()
	}
	if s.isoStorageField != nil {
		px.ISOStorage = s.isoStorageField.Value()
	}
	if s.fcosField != nil {
		px.FCOSIso = s.fcosField.Value()
	}
	if s.bootstrapField != nil {
		px.Node = s.bootstrapField.Value()
	}

	if len(s.controlPlaneFields) > 0 {
		nodes := make([]string, len(s.controlPlaneFields))
		for i, f := range s.controlPlaneFields {
			nodes[i] = f.Value()
		}
		px.ControlPlaneNodes = nodes
	}
	if len(s.workerFields) > 0 {
		nodes := make([]string, len(s.workerFields))
		for i, f := range s.workerFields {
			nodes[i] = f.Value()
		}
		px.WorkerNodes = nodes
	}
	return nil
}

// SetFocused propagates focus to the inner form.
func (s *NodePlacementStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if s.inner == nil {
		return
	}
	if focused {
		_ = s.inner.Focus() // cmd runs via Init(), not here
		return
	}
	s.inner.Blur()
}

// ShortHelp returns the step's help bar or nil while discovering.
func (s *NodePlacementStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == phaseDiscovering {
		return nil
	}
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: wizard.HelpNavigate},
		{Key: "← →", Help: "change value"},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
	}
}

func bridgeNames(bridges []proxmoxBridge) []string {
	names := make([]string, len(bridges))
	for i, b := range bridges {
		names[i] = b.Name
	}
	return names
}

func additionalNetworksBridges(nets []config.AdditionalNetwork) string {
	names := make([]string, len(nets))
	for i, n := range nets {
		names[i] = n.Bridge
	}
	return strings.Join(names, ",")
}

// parseAdditionalNetworks converts a bridge-name CSV to []AdditionalNetwork
// (nil if empty), preserving existing entries by Bridge match and defaulting
// new ones to Model "virtio".
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
