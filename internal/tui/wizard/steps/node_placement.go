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

// NodePlacementStep discovers the Proxmox nodes and lets the operator pick
// the pool and a host per node, written as a nodes.<name>.host override only
// where it differs from the model's placement.
type NodePlacementStep struct {
	wizard.BaseStep

	cfg      *config.Config
	phase    placementPhase
	frame    uint64
	capacity *WizardCapacitySnapshot
	secrets  func() wizard.Secrets
	saved    *credentials.ProxmoxCredentials

	discovery    *proxmoxDiscovery
	discoveryErr error
	// generation tags every fetch so a superseded reply is discarded unapplied
	generation uint64

	header     string
	inner      *wizard.MultiSectionForm
	poolField  *components.MultiSelectField
	addField   *components.InputField
	hostFields map[string]*components.SelectField
	inventory  []model.NodeSpec
	buildErr   error
}

// NewNodePlacementStep constructs the node placement step.
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

func (s *NodePlacementStep) withConfig(cfg *config.Config) *NodePlacementStep {
	s.cfg = cfg
	return s
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

// ShouldShow shows the step only for the Proxmox provider.
func (s *NodePlacementStep) ShouldShow(cfg *config.Config) bool {
	return cfg.Provider.Type == config.ProviderProxmox
}

// Init starts discovery, or after a success rebuilds the form from the
// cached inventory; a failed attempt always retries.
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

// startDiscovery resolves credentials now so the tea.Cmd never touches s.
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

// credentials copies the saved sidecar, then prefers a token typed this
// session and takes the TLS toggle as typed, since the proxmox step seeds it
// from the same sidecar.
func (s *NodePlacementStep) credentials() *credentials.ProxmoxCredentials {
	creds := &credentials.ProxmoxCredentials{Endpoint: s.cfg.Provider.Proxmox.Endpoint}
	if s.saved != nil {
		creds.APIToken = bytes.Clone(s.saved.APIToken)
		creds.Insecure = s.saved.Insecure
	}
	if s.secrets == nil {
		return creds
	}
	typed := s.secrets()
	defer typed.ProxmoxToken.Zeroize()
	creds.Insecure = typed.ProxmoxInsecure
	if !typed.ProxmoxToken.IsEmpty() {
		creds.Zeroize()
		creds.APIToken = bytes.Clone(typed.ProxmoxToken.Bytes())
	}
	return creds
}

// sanitizeDiscovery strips terminal escapes once, at the boundary.
func sanitizeDiscovery(disc *proxmoxDiscovery) *proxmoxDiscovery {
	if disc == nil {
		return nil
	}
	for i := range disc.Nodes {
		disc.Nodes[i].Name = tui.SanitizeTerminalEscapes(disc.Nodes[i].Name)
	}
	return disc
}

// hostNames returns the discovered names, or the config's pool when discovery
// failed so placement stays editable offline.
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

// buildInnerStep builds the pool multi-select, the add-nodes field and one
// host select per derived node, defaulting each to the model's placement
// over the offered pool.
func (s *NodePlacementStep) buildInnerStep(disc *proxmoxDiscovery, hosts []string) {
	pool := offeredPool(s.cfg.Provider.Proxmox.Nodes, hosts)
	options := slices.Clone(hosts)
	for _, name := range pool {
		if !slices.Contains(options, name) {
			options = append(options, name)
		}
	}
	s.poolField = components.NewMultiSelectField("placement pool", hosts)
	s.poolField.Help = "proxmox nodes the vms spread across (provider.proxmox.nodes)"
	s.poolField.SetValue(strings.Join(pool, ","))
	s.addField = components.NewInputField("add nodes", "none")
	s.addField.Help = "comma-separated node names to add to the pool when discovery did not list them"
	sections := []wizard.FormSection{{Title: "proxmox nodes", Group: components.NewInputGroup(s.poolField, s.addField)}}

	s.hostFields = make(map[string]*components.SelectField)
	s.inventory = nil
	base := *s.cfg
	base.Provider.Proxmox.Nodes = pool
	inv, defaults, err := placementInventory(&base)
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
				field := components.NewSelectField(node.Name, options)
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

// offeredPool keeps the configured pool while discovery lists any of it; a
// pool with no discovered member is DefaultConfig's placeholder.
func offeredPool(configured, hosts []string) []string {
	for _, name := range configured {
		if slices.Contains(hosts, name) {
			return configured
		}
	}
	return hosts
}

// placementInventory derives cfg's nodes and the host each would get with no
// override.
func placementInventory(cfg *config.Config) (model.Inventory, map[string]string, error) {
	inv, err := model.Build(cfg, model.PreviewInfraID)
	if err != nil {
		return model.Inventory{}, nil, err
	}
	bare := *cfg
	bare.Nodes = make(map[string]config.NodeOverride, len(cfg.Nodes))
	for name, o := range cfg.Nodes {
		o.Host = ""
		bare.Nodes[name] = o
	}
	base, err := model.Build(&bare, model.PreviewInfraID)
	if err != nil {
		return model.Inventory{}, nil, err
	}
	defaults := make(map[string]string, len(base.Nodes))
	for i := range base.Nodes {
		defaults[base.Nodes[i].Name] = base.Nodes[i].Host
	}
	return inv, defaults, nil
}

// Update handles discovery results, frames and the placement form.
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
	// 'r' re-issues discovery unless the add-nodes field would take it as text.
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && !s.ConsumesTextInput() && key.Matches(keyMsg, key.NewBinding(key.WithKeys("r"))) {
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
	if len(s.pool()) == 0 {
		return s, tea.Batch(s.inner.Focus(), func() tea.Msg { return wizard.ErrorSetMsg{Error: errPoolEmpty} })
	}
	return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: s.ID()} }
}

var errPoolEmpty = errors.New("check or type at least one proxmox node for the placement pool")

func (s *NodePlacementStep) pool() []string {
	pool := splitList(s.poolField.Value())
	for _, name := range splitList(s.addField.Value()) {
		if !slices.Contains(pool, name) {
			pool = append(pool, name)
		}
	}
	return pool
}

// ConsumesTextInput reports whether the add-nodes field holds focus.
func (s *NodePlacementStep) ConsumesTextInput() bool {
	return s.inner != nil && s.inner.ConsumesTextInput()
}

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
	return fmt.Sprintf("%dc · %dg", node.CPUs, node.MemGB)
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

// assignmentDemand sums what the chosen hosts would carry and flags the ones
// over capacity.
func (s *NodePlacementStep) assignmentDemand(width int) string {
	if s.discovery == nil || len(s.inventory) == 0 {
		return ""
	}
	use := demandByHost(s.inventory, func(n *model.NodeSpec) string {
		if field := s.hostFields[n.Name]; field != nil {
			return field.Value()
		}
		return n.Host
	})
	rows := make([]string, 0, len(use))
	for i := range s.discovery.Nodes {
		node := &s.discovery.Nodes[i]
		d, assigned := use[node.Name]
		if !assigned {
			continue
		}
		row := fmt.Sprintf("%s: %dc/%dg/%dgb", tui.SanitizeTerminalEscapes(node.Name), d.cpu, d.memoryMiB/1024, d.diskGiB)
		if node.exceeds(d) {
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

// View renders the discovery spinner or the header and placement form.
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

// SetSize propagates the width to the placement form.
func (s *NodePlacementStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	if s.inner != nil {
		s.inner.SetWidth(width)
	}
}

// FocusBounds includes the discovery header above the current field.
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

// Apply writes the pool and a host override wherever a touched choice differs
// from the model's placement; an untouched host follows the pool and a host
// override for a node the config no longer produces is dropped.
func (s *NodePlacementStep) Apply(cfg *config.Config) error {
	if s.inner == nil {
		return nil
	}
	cfg.Provider.Proxmox.Nodes = s.pool()
	_, defaults, err := placementInventory(cfg)
	if err != nil {
		return nil //nolint:nilerr // without a buildable model there is no placement to write; cfg.Validate() reports the cause at review
	}
	pruneStaleHostOverrides(cfg, defaults)
	for name, field := range s.hostFields {
		def, produced := defaults[name]
		if !produced {
			continue
		}
		host := field.Value()
		if field.IsDefault() {
			host = def
		}
		setHostOverride(cfg, name, host, def)
	}
	return nil
}

// pruneStaleHostOverrides drops the host override of every node outside
// produced; other overrides stay for Validate to report.
func pruneStaleHostOverrides(cfg *config.Config, produced map[string]string) {
	for name, override := range cfg.Nodes {
		if _, ok := produced[name]; !ok && override.Host != "" {
			delete(cfg.Nodes, name)
		}
	}
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

// ShortHelp returns the help bar for the current phase.
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
