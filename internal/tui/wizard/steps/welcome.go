package steps

import (
	"errors"
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

// HubFlow builds one of the flows a hub verb swaps into. Building is deferred
// to the moment the verb is confirmed: the manage-nodes flow's hooks load
// credentials and probe the Proxmox host, work no plain `okdctl deploy` should
// pay for unless the operator asks for it.
type HubFlow func() ([]wizard.WizardStep, wizard.FlowChrome, error)

// HubFlows are the in-process flows the hub's verbs reach; a nil provider
// leaves its verb to the CLI, which handles it after the wizard exits.
type HubFlows struct {
	ManageNodes   HubFlow
	ClusterStatus HubFlow
}

// errEmptyHubFlow reports a hub flow provider that produced no screens.
var errEmptyHubFlow = errors.New("open flow: no screens to show")

// hubFlowFailedMsg carries a flow that could not be built back to the hub.
type hubFlowFailedMsg struct {
	err error
}

// WelcomeStep is the hero-hub: okdctl's single entry screen, pairing the
// block-letter wordmark with the verb menu every flow is reached from.
type WelcomeStep struct {
	wizard.BaseStep
	configExists bool
	saveSlot     string
	entries      []hubEntry
	nav          *components.CompactSelector
	flows        HubFlows

	// opening names the flow being assembled off the update loop, so a verb
	// whose hooks take a moment to build says so instead of looking wedged.
	opening string
	// frame is the shared clock's counter, animating the opening notice
	// while Animating reports true.
	frame uint64

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

// Animating reports whether the opening notice needs frame ticks.
func (s *WelcomeStep) Animating() bool {
	return s.opening != ""
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

// SuppressesBadge hides the chrome's version badge on the blank-slate hub,
// where the defaults-seed version would advertise a cluster that doesn't
// exist; the badge returns once a real configuration is loaded.
func (s *WelcomeStep) SuppressesBadge() bool {
	return !s.configExists
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

// SetFlows wires the in-process flows the manage-nodes and cluster-status verbs
// swap into.
func (s *WelcomeStep) SetFlows(flows HubFlows) {
	s.flows = flows
}

// Update moves the pointer down the verb menu (arrows remapped onto the
// selector's vertical bindings) and confirms on enter or space.
func (s *WelcomeStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case hubFlowFailedMsg:
		s.opening = ""
		err := msg.err
		return s, func() tea.Msg { return wizard.ErrorSetMsg{Error: err} }
	case tea.KeyPressMsg:
		if msg.Code == tea.KeyEnter || msg.Code == tea.KeySpace {
			cmd := s.confirm()
			return s, cmd
		}
		s.nav, _ = s.nav.Update(components.ArrowsAsVertical(msg))
	case wizard.FrameMsg:
		s.frame = msg.Frame
	}
	return s, nil
}

// confirm dispatches the highlighted verb: one with an in-process flow behind
// it swaps the wizard onto that flow, everything else completes the step and is
// handled by the configure flow or by the CLI once the wizard exits.
func (s *WelcomeStep) confirm() tea.Cmd {
	// A flow is already being assembled off the update loop. A second confirm
	// would build a second one and queue a second swap, and the wizard can only
	// hold one return target — so the first flow would become unreachable, with
	// its credentials already released by the session the second one displaced.
	if s.opening != "" {
		return nil
	}

	verb := s.SelectedVerb()
	flow := s.flowFor(verb)
	if flow == nil {
		return func() tea.Msg { return wizard.StepCompleteMsg{StepID: wizard.StepIDWelcome} }
	}

	s.opening = s.labelFor(verb)
	buildFlow := func() tea.Msg {
		flowSteps, chrome, err := flow()
		if err != nil {
			return hubFlowFailedMsg{err: err}
		}
		// A provider that returns nothing would have the wizard decline the
		// swap silently, leaving the notice up and every later confirm refused
		// by the guard above — a dead hub. Report it as the failure it is.
		if len(flowSteps) == 0 {
			return hubFlowFailedMsg{err: errEmptyHubFlow}
		}
		return wizard.SwapFlowMsg{Steps: flowSteps, Chrome: chrome}
	}
	return buildFlow
}

// flowFor returns the in-process flow behind verb, or nil when the verb has none.
func (s *WelcomeStep) flowFor(verb HubVerb) HubFlow {
	switch verb {
	case HubVerbManageNodes:
		return s.flows.ManageNodes
	case HubVerbClusterStatus:
		return s.flows.ClusterStatus
	default:
		return nil
	}
}

// labelFor returns verb's menu label.
func (s *WelcomeStep) labelFor(verb HubVerb) string {
	for _, e := range s.entries {
		if e.verb == verb {
			return e.label
		}
	}
	return ""
}

// SetFocused clears any in-flight open notice as the hub regains focus: the
// flow it was opening has since been entered and escaped.
func (s *WelcomeStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused {
		s.opening = ""
	}
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
	if s.opening != "" {
		parts = append(parts, "", wizard.Spinner(s.frame)+" "+tui.MutedStyle.Render("opening "+s.opening+"…"))
	}

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

// Validate always returns nil; the hub has no inputs to validate.
func (s *WelcomeStep) Validate() error {
	return nil
}

// ShortHelp returns the hub's help bar — the one screen whose enter label
// reads "start" rather than "continue"/"confirm" (see keymap_help.go): the
// hub begins the wizard, it doesn't advance or confirm anything already in
// progress.
func (s *WelcomeStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "choose"},
		{Key: wizard.HelpEnter, Help: wizard.HelpStart},
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
