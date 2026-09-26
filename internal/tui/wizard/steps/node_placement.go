package steps

import (
	"errors"
	"fmt"
	"strings"

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

	frame        uint64
	discovery    *proxmoxDiscovery
	discoveryErr error
	capacity     *WizardCapacitySnapshot

	// header caches the last View's rendered discoveryHeader, so headerOffset
	// doesn't need the render width again.
	header string

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
	return &NodePlacementStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(
			wizard.StepIDNodePlacement,
			"node placement",
			"configure node placement",
			"auto-discovered from your proxmox cluster",
		),
		phase: phaseDiscovering,
	}
}

func (s *NodePlacementStep) withCapacity(snapshot *WizardCapacitySnapshot) *NodePlacementStep {
	s.capacity = snapshot
	return s
}

// Animating reports whether the discovery indicator needs frame ticks.
func (s *NodePlacementStep) Animating() bool {
	return s.phase == phaseDiscovering
}

// ShouldShow shows this step only when the Proxmox provider is selected.
func (s *NodePlacementStep) ShouldShow(cfg *config.Config) bool {
	if cfg.Provider.Type != config.ProviderProxmox {
		return false
	}
	s.cfg = cfg
	if s.capacity != nil {
		s.capacity.cfg = cfg
	}
	return true
}

// Init starts discovery, or — with no Proxmox provider configured — settles
// immediately into an explanatory error instead of spinning forever on a
// fetch that was never issued.
func (s *NodePlacementStep) Init() tea.Cmd {
	if s.cfg == nil || s.cfg.Provider.Proxmox == nil {
		s.phase = phasePlacing
		s.discoveryErr = errors.New("no proxmox provider configured — complete the proxmox step first")
		return nil
	}
	s.phase = phaseDiscovering
	return s.fetchDiscovery
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
		for _, field := range []*components.SelectField{s.osStorageField, s.dataStorageField, s.isoStorageField} {
			if field != nil {
				field.SetDisplayOptions(storageDisplayOptions(disc, field.Options))
			}
		}
	}

	defaultNode := nodeNames[0]

	s.bootstrapField = newSelectField(clusterName+"-bootstrap", "proxmox node for bootstrap vm",
		nodeNames, defaultNode, px.Node)
	if disc != nil {
		s.bootstrapField.SetDisplayOptions(nodeDisplayOptions(disc.Nodes, nodeNames))
	}
	sections = append(sections, wizard.FormSection{
		Title: "bootstrap",
		Group: components.NewInputGroup(s.bootstrapField),
	})

	if cpCount := s.cfg.Topology.ControlPlane.Count; cpCount > 0 {
		s.controlPlaneFields = nodeSelectFields(fieldPrefixMaster, clusterName, cpCount, px.ControlPlaneNodes, defaultNode, nodeNames)
		if disc != nil {
			setNodeDisplayOptions(s.controlPlaneFields, disc.Nodes, nodeNames)
		}
		sections = append(sections, wizard.FormSection{
			Title: roleLabelControlPlane,
			Group: selectFieldGroup(s.controlPlaneFields),
		})
	}

	if wCount := s.cfg.Topology.Workers.Count; wCount > 0 {
		s.workerFields = nodeSelectFields(fieldPrefixWorker, clusterName, wCount, px.WorkerNodes, defaultNode, nodeNames)
		if disc != nil {
			setNodeDisplayOptions(s.workerFields, disc.Nodes, nodeNames)
		}
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
		if s.capacity != nil {
			s.capacity.discovery = msg.discovery
		}

		var nodeNames []string
		if msg.err == nil && msg.discovery != nil && len(msg.discovery.Nodes) > 0 {
			nodeNames = make([]string, len(msg.discovery.Nodes))
			for i := range msg.discovery.Nodes {
				nodeNames[i] = msg.discovery.Nodes[i].Name
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

	case wizard.FrameMsg:
		s.frame = msg.Frame
	}

	if s.phase == phasePlacing && s.inner != nil {
		cmd, enterPressed := s.inner.Update(msg)
		if !enterPressed {
			return s, cmd
		}

		s.inner.TouchAll()
		if errs := s.inner.Validate(); len(errs) > 0 {
			return s, tea.Batch(s.inner.FocusFirstInvalid(), func() tea.Msg { return wizard.ErrorSetMsg{Error: wizard.ErrFixHighlighted} })
		}
		return s, func() tea.Msg {
			return wizard.StepCompleteMsg{StepID: s.ID()}
		}
	}

	return s, nil
}

// discoveryHeader renders the discovery summary (or its failure) shown above
// the placement form, wrapped to width-2 to match the form's section rows,
// without the blank row that separates the two.
func (s *NodePlacementStep) discoveryHeader(width int) string {
	noteStyle := lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Italic(true).PaddingLeft(2)
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning()).PaddingLeft(2)

	switch {
	case s.discoveryErr != nil:
		return warnStyle.Width(width - 2).Render(s.discoveryErr.Error())
	case s.discovery != nil:
		header := noteStyle.Width(width - 2).Render(fmt.Sprintf("discovered %d node(s), %d storage pool(s), %d bridge(s)",
			len(s.discovery.Nodes), len(s.discovery.Storage), len(s.discovery.Bridges)))
		for nodeIndex := range s.discovery.Nodes {
			node := &s.discovery.Nodes[nodeIndex]
			cpu, memory := "?c", "?g"
			if node.CPUsKnown {
				cpu = fmt.Sprintf("%dc", node.CPUs)
			}
			if node.MemKnown {
				memory = fmt.Sprintf("%dg", node.MemGB)
			}
			status := lipgloss.NewStyle().Foreground(tui.ColorSuccess()).Render(tui.IconSuccess + " online")
			if node.Status != proxmoxStatusOnline {
				status = lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render("offline")
			}
			header += "\n" + noteStyle.Width(width-2).Render(fmt.Sprintf("%s %s · %s · %s", node.Name, status, cpu, memory))
		}
		if demand := s.assignmentDemand(width - 2); demand != "" {
			header += "\n" + demand
		}
		if s.discovery.Heterogeneous {
			header += "\n" + warnStyle.Width(width-2).
				Render(tui.IconWarning+" node inventories differ — offering only storage, bridges, and isos every online node shares")
		}
		return header
	default:
		return ""
	}
}

func nodeDisplayOptions(nodes []proxmoxNode, values []string) []string {
	byName := make(map[string]*proxmoxNode, len(nodes))
	for nodeIndex := range nodes {
		node := &nodes[nodeIndex]
		byName[node.Name] = node
	}
	display := make([]string, len(values))
	for i, name := range values {
		node, ok := byName[name]
		if !ok {
			display[i] = name
			continue
		}
		cpu, memory := "?c", "?g"
		if node.CPUsKnown {
			cpu = fmt.Sprintf("%dc", node.CPUs)
		}
		if node.MemKnown {
			memory = fmt.Sprintf("%dg", node.MemGB)
		}
		display[i] = fmt.Sprintf("%s — %s/%s", name, cpu, memory)
		if node.Status != proxmoxStatusOnline {
			display[i] += " " + lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render("offline")
		}
	}
	return display
}

func setNodeDisplayOptions(fields []*components.SelectField, nodes []proxmoxNode, values []string) {
	options := nodeDisplayOptions(nodes, values)
	for _, field := range fields {
		field.SetDisplayOptions(options)
	}
}

func storageDisplayOptions(discovery *proxmoxDiscovery, values []string) []string {
	display := make([]string, len(values))
	for i, name := range values {
		parts := make([]string, 0, len(discovery.Nodes))
		for nodeIndex := range discovery.Nodes {
			node := &discovery.Nodes[nodeIndex]
			if node.Status != proxmoxStatusOnline {
				continue
			}
			if !node.StorageKnown {
				parts = append(parts, node.Name+" ?")
				continue
			}
			for _, pool := range node.Storage {
				if pool.Name == name {
					parts = append(parts, node.Name+" "+formatStorageGB(pool.TotalGB, pool.TotalKnown))
					break
				}
			}
		}
		if len(parts) == 0 {
			display[i] = name + " — capacity unknown"
		} else {
			display[i] = name + " — " + strings.Join(parts, " · ")
		}
	}
	return display
}

func formatStorageGB(gigabytes int, known bool) string {
	if !known {
		return "?"
	}
	if gigabytes >= 1000 {
		return fmt.Sprintf("%.1ftb", float64(gigabytes)/1000)
	}
	return fmt.Sprintf("%dgb", gigabytes)
}

func (s *NodePlacementStep) assignmentDemand(width int) string {
	if s.cfg == nil || s.discovery == nil {
		return ""
	}
	demand := make(map[string][3]int)
	add := func(node string, cpu, memory, disk int) {
		current := demand[node]
		current[0] += cpu
		current[1] += memory
		current[2] += disk
		demand[node] = current
	}
	bootstrap := s.cfg.Topology.Bootstrap
	if bootstrap.CPU == 0 && bootstrap.MemoryMB == 0 {
		bootstrap.CPU = s.cfg.Topology.ControlPlane.CPU
		bootstrap.MemoryMB = s.cfg.Topology.ControlPlane.MemoryMB
	}
	if bootstrap.DiskGB == 0 {
		bootstrap.DiskGB = s.cfg.Topology.ControlPlane.DiskGB
	}
	if s.bootstrapField != nil {
		add(s.bootstrapField.Value(), bootstrap.CPU, bootstrap.MemoryMB/1024, bootstrap.DiskGB)
	}
	for i, field := range s.controlPlaneFields {
		if i < s.cfg.Topology.ControlPlane.Count {
			add(field.Value(), s.cfg.Topology.ControlPlane.CPU, s.cfg.Topology.ControlPlane.MemoryMB/1024,
				s.cfg.Topology.ControlPlane.DiskGB+s.cfg.Disks.ControlPlaneDataSizeGB)
		}
	}
	for i, field := range s.workerFields {
		if i < s.cfg.Topology.Workers.Count {
			add(field.Value(), s.cfg.Topology.Workers.CPU, s.cfg.Topology.Workers.MemoryMB/1024,
				s.cfg.Topology.Workers.DiskGB+s.cfg.Disks.WorkerDataSizeGB)
		}
	}
	rows := make([]string, 0, len(demand))
	for nodeIndex := range s.discovery.Nodes {
		node := &s.discovery.Nodes[nodeIndex]
		used, assigned := demand[node.Name]
		if !assigned {
			continue
		}
		row := fmt.Sprintf("%s: %dc/%dg/%dgb", node.Name, used[0], used[1], used[2])
		if (node.CPUsKnown && used[0] > node.CPUs) || (node.MemKnown && used[1] > node.MemGB) {
			row = lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render(row + " · oversubscribed")
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(max(1, width)).PaddingLeft(2).
		Render("assigned demand · " + strings.Join(rows, " · "))
}

// headerOffset is how many lines View prepends before the inner form's own
// line 0, so the form's spans can be rebased onto the step's View.
func (s *NodePlacementStep) headerOffset() int {
	if s.header == "" {
		return 0
	}
	return lipgloss.Height(s.header) + 1
}

// View renders either the loading spinner or the inner placement form.
func (s *NodePlacementStep) View(width, height int) string {
	s.SetSize(width, height)

	if s.phase == phaseDiscovering {
		return wizard.Spinner(s.frame) + " discovering proxmox infrastructure..."
	}

	s.header = s.discoveryHeader(width)
	header := s.header
	if header != "" {
		header += "\n\n"
	}

	if s.inner != nil {
		return header + s.inner.View(width)
	}
	return header
}

// FocusedSpan rebases the inner form's span onto this step's View, which
// prepends the discovery header.
func (s *NodePlacementStep) FocusedSpan() (wizard.LineSpan, bool) {
	if s.inner == nil || s.phase != phasePlacing {
		return wizard.LineSpan{}, false
	}
	span, ok := s.inner.FocusedSpan()
	if !ok {
		return wizard.LineSpan{}, false
	}
	offset := s.headerOffset()
	return wizard.LineSpan{Start: span.Start + offset, End: span.End + offset}, true
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

// PaletteTargets exposes discovered placement fields without their values.
func (s *NodePlacementStep) PaletteTargets() []wizard.PaletteTarget {
	if s.inner == nil {
		return nil
	}
	return s.inner.PaletteTargets()
}

// FocusPaletteTarget moves focus to a discovered placement field.
func (s *NodePlacementStep) FocusPaletteTarget(id string) tea.Cmd {
	if s.inner == nil {
		return nil
	}
	return s.inner.FocusPaletteTarget(id)
}

// ShortHelp returns the step's help bar — {esc back, ctrl+c quit} while
// discovering, else the placement form's bindings plus any key hints the
// focused field contributes.
func (s *NodePlacementStep) ShortHelp() []wizard.KeyBinding {
	if s.phase == phaseDiscovering {
		return []wizard.KeyBinding{
			{Key: wizard.HelpEsc, Help: wizard.HelpBack},
			{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
		}
	}
	help := []wizard.KeyBinding{
		{Key: "↑↓", Help: wizard.HelpNavigate},
		{Key: "← →", Help: "change value"},
		{Key: wizard.HelpEnter, Help: wizard.HelpContinue},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
	if s.inner == nil {
		return help
	}
	if h, ok := s.inner.FocusedField().(components.KeyHinter); ok {
		for _, hint := range h.KeyHints() {
			help = append(help, wizard.KeyBinding{Key: hint.Key, Help: hint.Help})
		}
	}
	return help
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
