package steps

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/model"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

type placementPhase int

const (
	phaseDiscovering placementPhase = iota
	phasePlacing
)

type discoveryCompleteMsg struct {
	generation uint64
	discovery  *proxmoxDiscovery
	err        error
}

// NodePlacementStep discovers the Proxmox nodes, then lets the operator pick
// the placement pool and a host per node; a host is written as a
// nodes.<name>.host override only where it differs from the model's own
// placement.
type NodePlacementStep struct {
	wizard.BaseStep

	cfg      *config.Config
	phase    placementPhase
	frame    uint64
	capacity *WizardCapacitySnapshot
	secrets  func() wizard.Secrets

	discovery    *proxmoxDiscovery
	discoveryErr error
	// generation tags every fetch so a superseded reply is discarded unapplied.
	generation uint64

	header     string
	inner      *wizard.MultiSectionForm
	poolField  *components.MultiSelectField
	addField   *components.InputField
	hostFields map[string]*components.SelectField
	inventory  []model.NodeSpec
	buildErr   error
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

func (s *NodePlacementStep) withSecrets(fn func() wizard.Secrets) *NodePlacementStep {
	s.secrets = fn
	return s
}

// Animating reports whether the discovery indicator needs frame ticks.
func (s *NodePlacementStep) Animating() bool {
	return s.phase == phaseDiscovering
}

// ShouldShow shows this step only for the Proxmox provider and records cfg.
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

// Init starts discovery, or on re-entry after a success rebuilds the form
// from the cached inventory so a changed worker count or cluster name is
// reflected without another fetch; a failed attempt always retries.
func (s *NodePlacementStep) Init() tea.Cmd {
	if s.phase == phasePlacing && s.discoveryErr == nil && s.discovery != nil {
		s.buildInnerStep(s.discovery, s.hostNames())
		return s.inner.Init()
	}
	if s.cfg == nil || s.cfg.Provider.Proxmox.Endpoint == "" {
		s.phase = phasePlacing
		s.discoveryErr = errors.New("no proxmox endpoint configured — complete the proxmox step first")
		return nil
	}
	return s.startDiscovery()
}

// startDiscovery issues a generation-tagged fetch over credentials resolved
// now, so the returned tea.Cmd never touches s once handed to bubbletea.
func (s *NodePlacementStep) startDiscovery() tea.Cmd {
	s.phase = phaseDiscovering
	s.discoveryErr = nil
	s.generation++
	generation := s.generation
	parent := s.Context()
	creds := s.credentials()
	return func() tea.Msg {
		defer creds.Zeroize()
		disc, err := discoverProxmox(parent, creds)
		return discoveryCompleteMsg{generation: generation, discovery: disc, err: err}
	}
}

// credentials prefers a token typed this session over the environment's.
func (s *NodePlacementStep) credentials() *credentials.ProxmoxCredentials {
	creds := credentials.GetProxmoxCredentials(s.cfg)
	if s.secrets == nil {
		return creds
	}
	typed := s.secrets()
	defer typed.ProxmoxToken.Zeroize()
	creds.Insecure = creds.Insecure || typed.ProxmoxInsecure
	if !typed.ProxmoxToken.IsEmpty() {
		creds.Zeroize()
		creds.APIToken = bytes.Clone(typed.ProxmoxToken.Bytes())
	}
	return creds
}

// sanitizeDiscovery strips terminal escapes from the node names once, at the
// boundary, since the pool multi-select shows its options verbatim.
func sanitizeDiscovery(disc *proxmoxDiscovery) *proxmoxDiscovery {
	if disc == nil {
		return nil
	}
	for i := range disc.Nodes {
		disc.Nodes[i].Name = tui.SanitizeTerminalEscapes(disc.Nodes[i].Name)
	}
	return disc
}

// hostNames returns the discovered node names, or the config's pool when
// discovery failed, so placement stays editable offline.
func (s *NodePlacementStep) hostNames() []string {
	if s.discovery != nil && len(s.discovery.Nodes) > 0 {
		names := make([]string, len(s.discovery.Nodes))
		for i := range s.discovery.Nodes {
			names[i] = s.discovery.Nodes[i].Name
		}
		return names
	}
	if len(s.cfg.Provider.Proxmox.Nodes) > 0 {
		return s.cfg.Provider.Proxmox.Nodes
	}
	return []string{"pve"}
}

// buildInnerStep builds the pool multi-select, a typed-names field for nodes
// discovery did not list, and one host select per node the model derives,
// defaulting each to the model's placement.
func (s *NodePlacementStep) buildInnerStep(disc *proxmoxDiscovery, hosts []string) {
	s.poolField = components.NewMultiSelectField("placement pool", hosts)
	s.poolField.Help = "proxmox nodes the vms spread across (provider.proxmox.nodes)"
	s.poolField.SetValue(strings.Join(s.cfg.Provider.Proxmox.Nodes, ","))
	s.addField = components.NewInputField("add nodes", "none")
	s.addField.Help = "comma-separated node names to add to the pool when discovery did not list them"
	sections := []wizard.FormSection{{Title: "proxmox nodes", Group: components.NewInputGroup(s.poolField, s.addField)}}

	s.hostFields = make(map[string]*components.SelectField)
	s.inventory = nil
	inv, defaults, err := placementInventory(s.cfg)
	s.buildErr = err
	if err == nil {
		s.inventory = inv.Nodes
		for _, role := range []struct {
			title string
			nodes []model.NodeSpec
		}{{roleLabelControlPlane, inv.Masters()}, {roleLabelWorkers, inv.Workers()}} {
			if len(role.nodes) == 0 {
				continue
			}
			fields := make([]components.FormField, 0, len(role.nodes))
			for i := range role.nodes {
				node := &role.nodes[i]
				field := components.NewSelectField(node.Name, hosts)
				field.Help = "proxmox node (nodes." + node.Name + ".host)"
				field.SetDefault(defaults[node.Name])
				if override := s.cfg.Nodes[node.Name].Host; override != "" {
					field.SetValue(override)
				}
				if disc != nil {
					field.SetDisplayOptions(nodeDisplayOptions(disc.Nodes, field.Options))
				}
				s.hostFields[node.Name] = field
				fields = append(fields, field)
			}
			sections = append(sections, wizard.FormSection{Title: role.title, Group: components.NewInputGroup(fields...)})
		}
	}
	s.inner = wizard.NewMultiSectionForm(sections)
}

// placementInventory derives cfg's nodes plus the host each would get with
// no override, the baseline a chosen host is compared against.
func placementInventory(cfg *config.Config) (model.Inventory, map[string]string, error) {
	inv, err := model.Build(cfg, "")
	if err != nil {
		return model.Inventory{}, nil, err
	}
	bare := *cfg
	bare.Nodes = make(map[string]config.NodeOverride, len(cfg.Nodes))
	for name, o := range cfg.Nodes {
		o.Host = ""
		bare.Nodes[name] = o
	}
	base, err := model.Build(&bare, "")
	if err != nil {
		return model.Inventory{}, nil, err
	}
	defaults := make(map[string]string, len(base.Nodes))
	for i := range base.Nodes {
		defaults[base.Nodes[i].Name] = base.Nodes[i].Host
	}
	return inv, defaults, nil
}

// Update handles discovery results, frame ticks and the placement form.
func (s *NodePlacementStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case discoveryCompleteMsg:
		if msg.generation != s.generation {
			return s, nil
		}
		s.discovery = sanitizeDiscovery(msg.discovery)
		s.discoveryErr = msg.err
		s.phase = phasePlacing
		if s.capacity != nil {
			s.capacity.discovery = msg.discovery
		}
		s.buildInnerStep(s.discovery, s.hostNames())
		return s, s.inner.Init()

	case wizard.FrameMsg:
		s.frame = msg.Frame
	}

	if s.phase != phasePlacing || s.inner == nil {
		return s, nil
	}
	// 'r' re-issues discovery, footer-silent like the vim scroll keys.
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, key.NewBinding(key.WithKeys("r"))) {
		cmd := s.startDiscovery()
		return s, cmd
	}
	cmd, enterPressed := s.inner.Update(msg)
	if !enterPressed {
		return s, cmd
	}
	s.inner.TouchAll()
	if errs := s.inner.Validate(); len(errs) > 0 {
		return s, tea.Batch(s.inner.FocusFirstInvalid(), func() tea.Msg { return wizard.ErrorSetMsg{Error: wizard.ErrFixHighlighted} })
	}
	return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: s.ID()} }
}

// discoveryHeader renders the discovery summary or its failure above the form.
func (s *NodePlacementStep) discoveryHeader(width int) string {
	noteStyle := lipgloss.NewStyle().Foreground(tui.ColorTextFaint()).Italic(true).PaddingLeft(2)
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning()).PaddingLeft(2)

	var lines []string
	switch {
	case s.discoveryErr != nil:
		lines = append(lines, warnStyle.Width(width-2).Render(tui.SanitizeTerminalEscapes(s.discoveryErr.Error())))
	case s.discovery != nil:
		lines = append(lines, noteStyle.Width(width-2).Render(fmt.Sprintf("discovered %d node(s)", len(s.discovery.Nodes))))
		for i := range s.discovery.Nodes {
			node := &s.discovery.Nodes[i]
			status := lipgloss.NewStyle().Foreground(tui.ColorSuccess()).Render(tui.IconSuccess + " online")
			if node.Status != proxmoxStatusOnline {
				status = lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render("offline")
			}
			lines = append(lines, noteStyle.Width(width-2).Render(fmt.Sprintf("%s %s · %s", tui.SanitizeTerminalEscapes(node.Name), status, nodeCapacity(node))))
		}
		if demand := s.assignmentDemand(width - 2); demand != "" {
			lines = append(lines, demand)
		}
	}
	if s.buildErr != nil {
		lines = append(lines, warnStyle.Width(width-2).Render("node placement unavailable: "+tui.SanitizeTerminalEscapes(s.buildErr.Error())))
	}
	return strings.Join(lines, "\n")
}

func nodeCapacity(node *proxmoxNode) string {
	cpu, memory := "?c", "?g"
	if node.CPUsKnown {
		cpu = fmt.Sprintf("%dc", node.CPUs)
	}
	if node.MemKnown {
		memory = fmt.Sprintf("%dg", node.MemGB)
	}
	return cpu + " · " + memory
}

func nodeDisplayOptions(nodes []proxmoxNode, values []string) []string {
	byName := make(map[string]*proxmoxNode, len(nodes))
	for i := range nodes {
		byName[nodes[i].Name] = &nodes[i]
	}
	display := make([]string, len(values))
	for i, name := range values {
		node, ok := byName[name]
		if !ok {
			display[i] = tui.SanitizeTerminalEscapes(name)
			continue
		}
		display[i] = fmt.Sprintf("%s — %s", tui.SanitizeTerminalEscapes(name), strings.ReplaceAll(nodeCapacity(node), " · ", "/"))
		if node.Status != proxmoxStatusOnline {
			display[i] += " " + lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render("offline")
		}
	}
	return display
}

// assignmentDemand sums the vcpu, memory and disk the chosen hosts would
// carry and flags the ones over their discovered capacity.
func (s *NodePlacementStep) assignmentDemand(width int) string {
	if s.discovery == nil || len(s.inventory) == 0 {
		return ""
	}
	type demand struct{ cpu, memGB, diskGB int }
	use := make(map[string]*demand)
	for i := range s.inventory {
		node := &s.inventory[i]
		host := node.Host
		if field := s.hostFields[node.Name]; field != nil {
			host = field.Value()
		}
		d := use[host]
		if d == nil {
			d = &demand{}
			use[host] = d
		}
		d.cpu += node.CPU
		d.memGB += node.MemoryMiB / 1024
		for _, disk := range node.Disks {
			d.diskGB += disk.SizeGiB
		}
	}
	rows := make([]string, 0, len(use))
	for i := range s.discovery.Nodes {
		node := &s.discovery.Nodes[i]
		d, assigned := use[node.Name]
		if !assigned {
			continue
		}
		row := fmt.Sprintf("%s: %dc/%dg/%dgb", tui.SanitizeTerminalEscapes(node.Name), d.cpu, d.memGB, d.diskGB)
		if (node.CPUsKnown && d.cpu > node.CPUs) || (node.MemKnown && d.memGB > node.MemGB) {
			row = lipgloss.NewStyle().Foreground(tui.ColorWarning()).Render(row + " · oversubscribed")
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return ""
	}
	text := tui.Truncate("assigned demand · "+strings.Join(rows, " · "), max(1, width-2))
	return lipgloss.NewStyle().PaddingLeft(2).Render(text)
}

func (s *NodePlacementStep) headerOffset() int {
	if s.header == "" {
		return 0
	}
	return lipgloss.Height(s.header) + 1
}

// View renders the discovery spinner or the header plus the placement form.
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

// SetSize propagates the viewport width to the placement form.
func (s *NodePlacementStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	if s.inner != nil {
		s.inner.SetWidth(width)
	}
}

// FocusBounds includes the discovery summary above the current field.
func (s *NodePlacementStep) FocusBounds(_, _ int) (top, bottom int, ok bool) {
	if s.inner == nil || s.phase == phaseDiscovering {
		return 0, 0, false
	}
	top, bottom, ok = s.inner.FocusBounds()
	offset := s.headerOffset()
	return top + offset, bottom + offset, ok
}

// FocusedSpan rebases the form's span onto this step's View.
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

// Apply writes the pool (the checked nodes plus any typed ones) and, per
// node the current config produces, a host override only where the choice
// differs from the model's placement.
func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if s.inner == nil {
		return nil
	}
	pool := splitList(s.poolField.Value())
	for _, name := range splitList(s.addField.Value()) {
		if !slices.Contains(pool, name) {
			pool = append(pool, name)
		}
	}
	if len(pool) > 0 {
		cfg.Provider.Proxmox.Nodes = pool
	}
	_, defaults, err := placementInventory(cfg)
	if err != nil {
		return nil //nolint:nilerr // without a buildable model there is no placement to write; cfg.Validate() reports the cause at review
	}
	for name, field := range s.hostFields {
		def, produced := defaults[name]
		if !produced {
			continue
		}
		setHostOverride(cfg, name, field.Value(), def)
	}
	return nil
}

func setHostOverride(cfg *config.Config, name, host, def string) {
	override := cfg.Nodes[name]
	override.Host = host
	if host == def {
		override.Host = ""
	}
	if override.Host == "" && override.IP == "" && override.CPU == nil && override.MemoryMiB == nil && len(override.Disks) == 0 {
		delete(cfg.Nodes, name)
		return
	}
	if cfg.Nodes == nil {
		cfg.Nodes = make(map[string]config.NodeOverride)
	}
	cfg.Nodes[name] = override
}

// SetFocused propagates focus to the inner form.
func (s *NodePlacementStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if s.inner == nil {
		return
	}
	if focused {
		_ = s.inner.Focus()
		return
	}
	s.inner.Blur()
}

// ShortHelp returns the step's help bar for the current phase.
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

// OverlayHelp lists the footer-silent "r" refresh once a form is built.
func (s *NodePlacementStep) OverlayHelp() []wizard.KeyBinding {
	if s.phase != phasePlacing || s.inner == nil {
		return nil
	}
	return []wizard.KeyBinding{{Key: "r", Help: helpRefresh}}
}
