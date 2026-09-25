package steps

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// HubVerb names one entry of the hub's menu — the single action the operator
// picked on the welcome screen.
type HubVerb int

// The hub's verbs: the first five are the menu shown over an existing
// configuration, the last two the blank-slate menu.
const (
	HubVerbDeploy HubVerb = iota
	HubVerbEditConfig
	HubVerbManageNodes
	HubVerbClusterStatus
	HubVerbDestroy
	HubVerbGetStarted
	HubVerbQuit
)

// SaveSlotState is how far the loaded configuration has got, as far as the hub
// can honestly tell from local files alone.
type SaveSlotState string

// SaveSlotState values, ordered by how much of a cluster each implies exists.
const (
	SaveSlotConfigured SaveSlotState = "configured"
	SaveSlotDeploying  SaveSlotState = "deploying"
	SaveSlotDeployed   SaveSlotState = "deployed"
)

// hubEntry pairs a menu label with the verb it selects.
type hubEntry struct {
	verb  HubVerb
	label string
}

// hubVerbs is the menu shown when an okdctl.yaml exists: five verbs, in the
// order the operator is most likely to want them.
var hubVerbs = []hubEntry{
	{HubVerbDeploy, "deploy"},
	{HubVerbEditConfig, "edit config"},
	{HubVerbManageNodes, "manage nodes"},
	{HubVerbClusterStatus, "cluster status"},
	{HubVerbDestroy, "destroy"},
}

// hubFreshVerbs is the blank-slate menu: there is nothing to deploy, manage,
// inspect or destroy yet.
var hubFreshVerbs = []hubEntry{
	{HubVerbGetStarted, "get started"},
	{HubVerbQuit, "quit"},
}

// WelcomeStep is the hero-hub: okdctl's single entry screen, pairing the
// block-letter wordmark with the verb menu every flow is reached from.
type WelcomeStep struct {
	wizard.BaseStep
	configExists bool
	saveSlot     string
	entries      []hubEntry
	nav          *components.CompactSelector

	// termWidth and termHeight are the terminal's own dimensions, not the
	// content box View renders into — the hero's double-scale gate is stated
	// in terminal columns and rows.
	termWidth  int
	termHeight int
}

// NewWelcomeStep constructs the hub step on its blank-slate menu.
func NewWelcomeStep() *WelcomeStep {
	s := &WelcomeStep{
		BaseStep: wizard.NewBaseStep(wizard.StepIDWelcome, "welcome", ""),
	}
	s.setEntries(hubFreshVerbs)
	return s
}

// setEntries rebuilds the menu over entries, clamped so up/down never wraps
// past either end of a launcher's short list.
func (s *WelcomeStep) setEntries(entries []hubEntry) {
	s.entries = entries
	labels := make([]string, len(entries))
	for i, e := range entries {
		labels[i] = e.label
	}
	s.nav = components.NewCompactSelector(labels)
	s.nav.SetWrap(false)
}

// SetConfigExists tells the hub whether okdctl.yaml exists, switching between
// the five-verb and blank-slate menus.
func (s *WelcomeStep) SetConfigExists(exists bool) {
	s.configExists = exists
	if exists {
		s.setEntries(hubVerbs)
		return
	}
	// The blank slate has no slot to describe; a line left over from an
	// earlier SetExistingConfig would name a configuration that is gone.
	s.saveSlot = ""
	s.setEntries(hubFreshVerbs)
}

// SetExistingConfig switches the hub to its five-verb menu and derives the dim
// save-slot line from cfg and the state the caller could honestly determine.
func (s *WelcomeStep) SetExistingConfig(cfg *config.Config, state SaveSlotState) {
	s.SetConfigExists(true)
	if cfg == nil {
		return
	}
	nodes := cfg.Topology.ControlPlane.Count + cfg.Topology.Workers.Count
	s.saveSlot = fmt.Sprintf("%s · okd %s · %s · %s",
		cfg.Cluster.Name, cfg.Distribution.Version, pluralNodes(nodes), state)
}

// pluralNodes renders a node count with its noun agreeing.
func pluralNodes(n int) string {
	if n == 1 {
		return "1 node"
	}
	return fmt.Sprintf("%d nodes", n)
}

// SelectedVerb returns the verb the operator has highlighted.
func (s *WelcomeStep) SelectedVerb() HubVerb {
	i := s.nav.SelectedIndex()
	if i < 0 || i >= len(s.entries) {
		return HubVerbQuit
	}
	return s.entries[i].verb
}

// Init returns nil; the hub has no async startup work.
func (s *WelcomeStep) Init() tea.Cmd {
	return nil
}

// Update moves the pointer down the verb menu (arrows remapped onto the
// selector's vertical bindings) and confirms on enter or space.
func (s *WelcomeStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	if keyMsg.Code == tea.KeyEnter || keyMsg.Code == tea.KeySpace {
		return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: wizard.StepIDWelcome} }
	}
	s.nav, _ = s.nav.Update(components.ArrowsAsVertical(keyMsg))
	return s, nil
}

// IsCentered returns true so the launcher sits in the middle of the frame.
func (s *WelcomeStep) IsCentered() bool {
	return true
}

// RendersHero returns true so the frame drops its own header: the block-letter
// wordmark below is the screen's identity, and the configure flow's phase trail
// would describe a walkthrough four of the five verbs never enter.
func (s *WelcomeStep) RendersHero() bool {
	return true
}

// SuppressesSplit returns true so a wide terminal never puts a context pane
// beside the hub: a centered launcher owns its whole width, and the pane is a
// work-screen device with no work to describe here.
func (s *WelcomeStep) SuppressesSplit() bool {
	return true
}

// SetTerminalSize records the terminal's own dimensions, which gate the hero's
// double-scale rendering.
func (s *WelcomeStep) SetTerminalSize(width, height int) {
	s.termWidth, s.termHeight = width, height
}

// View renders the hero, the dim save-slot line when a configuration exists,
// and the verb menu — no tagline, no checklist columns, no per-verb copy.
func (s *WelcomeStep) View(width, height int) string {
	s.SetSize(width, height)

	parts := []string{renderHero(s.termWidth, s.termHeight, tui.ColorEnabled()), ""}
	if s.saveSlot != "" {
		parts = append(parts, tui.MutedStyle.Render(s.saveSlot), "")
	}
	parts = append(parts, s.nav.ViewPointer())

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

// Validate always returns nil; the hub has no inputs to validate.
func (s *WelcomeStep) Validate() error {
	return nil
}

// ShortHelp returns the hub's help bar.
func (s *WelcomeStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "choose"},
		{Key: wizard.HelpEnter, Help: "go"},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}

// GetSelectedAction maps the highlighted verb to the wizard's terminal action.
func (s *WelcomeStep) GetSelectedAction() wizard.Action {
	if s.SelectedVerb() == HubVerbDeploy {
		return wizard.ActionDeploy
	}
	return wizard.ActionExit
}

// ShouldExitEarly reports whether the highlighted verb is handled outside the
// configure flow, so confirming it leaves the wizard rather than advancing.
func (s *WelcomeStep) ShouldExitEarly() bool {
	switch s.SelectedVerb() {
	case HubVerbEditConfig, HubVerbGetStarted:
		return false
	default:
		return true
	}
}
