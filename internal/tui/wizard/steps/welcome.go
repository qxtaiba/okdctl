package steps

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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
	HubVerbResumeDraft
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

// hubEntry pairs a menu label with the verb it selects; accel is the verb's
// single-key accelerator in review's [N] grammar, empty for verbs the
// friction ladder keeps behind pointer + enter.
type hubEntry struct {
	verb  HubVerb
	label string
	accel string
}

// hubVerbs is the menu shown when an okdctl.yaml exists: five verbs, in the
// order the operator is most likely to want them. Destroy deliberately has
// no accelerator — and no digit either, since digits address only entries
// that carry an accelerator.
var hubVerbs = []hubEntry{
	{HubVerbDeploy, labelDeploy, "d"},
	{HubVerbEditConfig, "edit config", "e"},
	{HubVerbManageNodes, "manage nodes", "n"},
	{HubVerbClusterStatus, "cluster status", "s"},
	{HubVerbDestroy, "destroy", ""},
}

// hubFreshVerbs is the blank-slate menu: there is nothing to deploy, manage,
// inspect or destroy yet.
var hubFreshVerbs = []hubEntry{
	{HubVerbGetStarted, "get started", ""},
	{HubVerbQuit, "quit", ""},
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

// WelcomeStep is okdctl's hub for cluster operations and configuration flows.
type WelcomeStep struct {
	wizard.BaseStep
	configExists  bool
	saveSlot      string
	draftLabel    string
	draftCursor   wizard.DraftResumeMsg
	entries       []hubEntry
	nav           *components.CompactSelector
	flows         HubFlows
	opsSource     StatusSource
	opsStatus     *opsSnapshot
	opsLatency    []opsLatencySample
	opsErr        error
	opsLoading    bool
	opsActive     bool
	opsGeneration uint64
	opsCtx        context.Context
	opsCancel     context.CancelFunc

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
// past either end of a launcher's short list. On a menu with accelerators,
// each row leads with its "[d]" hint and accelerator-less rows indent to
// keep the verbs on one column — the missing bracket is the signal.
func (s *WelcomeStep) setEntries(entries []hubEntry) {
	s.entries = entries
	hasAccels := false
	for _, e := range entries {
		if e.accel != "" {
			hasAccels = true
			break
		}
	}
	labels := make([]string, len(entries))
	for i, e := range entries {
		switch {
		case e.accel != "":
			labels[i] = "[" + e.accel + "] " + e.label
		case hasAccels:
			labels[i] = "    " + e.label
		default:
			labels[i] = e.label
		}
	}
	s.nav = components.NewCompactSelector(labels)
	s.nav.SetWrap(false)
}

// SetConfigExists tells the hub whether okdctl.yaml exists, switching between
// the five-verb and blank-slate menus.
func (s *WelcomeStep) SetConfigExists(exists bool) {
	s.configExists = exists
	entries := hubFreshVerbs
	if exists {
		entries = hubVerbs
	} else {
		// The blank slate has no slot to describe; a line left over from an
		// earlier SetExistingConfig would name a configuration that is gone.
		s.saveSlot = ""
	}
	if s.draftLabel != "" {
		entries = append([]hubEntry{{verb: HubVerbResumeDraft, label: s.draftLabel}}, entries...)
	}
	s.setEntries(entries)
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

// SetDraftResume adds a hub entry that resumes the saved configure cursor.
func (s *WelcomeStep) SetDraftResume(stepID wizard.StepID, fieldKey, label string) {
	s.draftCursor = wizard.DraftResumeMsg{StepID: stepID, FieldKey: fieldKey}
	s.draftLabel = label
	s.SetConfigExists(s.configExists)
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

// PaletteTargets exposes hub actions without dispatching them.
func (s *WelcomeStep) PaletteTargets() []wizard.PaletteTarget {
	targets := make([]wizard.PaletteTarget, 0, len(s.entries))
	for _, entry := range s.entries {
		targets = append(targets, wizard.PaletteTarget{
			ID:     strconv.Itoa(int(entry.verb)),
			Kind:   wizard.PaletteTargetAction,
			Label:  entry.label,
			Detail: "Select hub action",
		})
	}
	return targets
}

// FocusPaletteTarget highlights a hub action without confirming it.
func (s *WelcomeStep) FocusPaletteTarget(id string) tea.Cmd {
	for i, entry := range s.entries {
		if strconv.Itoa(int(entry.verb)) == id {
			s.nav.Select(i)
			break
		}
	}
	return nil
}

// Init starts the live snapshot and its cancellable refresh timer when enabled.
func (s *WelcomeStep) Init() tea.Cmd {
	if s.opsSource == nil {
		return nil
	}
	if s.opsCancel != nil {
		s.opsCancel()
	}
	// The hub owns its polling context and cancels it when focus leaves the hub.
	s.opsCtx, s.opsCancel = context.WithCancel(context.Background())
	s.opsActive = true
	s.opsGeneration++
	return tea.Batch(s.probeOps(s.opsGeneration), s.scheduleOpsRefresh(s.opsGeneration))
}

// SetOpsDashboard enables the live snapshot for a locally established cluster.
func (s *WelcomeStep) SetOpsDashboard(src StatusSource) {
	s.opsSource = src
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
	case opsSnapshotMsg:
		if !s.opsActive || msg.generation != s.opsGeneration {
			return s, nil
		}
		s.opsLoading = false
		s.opsErr = msg.err
		s.opsLatency = appendOpsLatency(s.opsLatency, opsLatencySample{
			duration:  msg.latency,
			available: msg.latencyAvailable,
		})
		if msg.err == nil {
			s.opsStatus = &opsSnapshot{
				status:           msg.status,
				updated:          time.Now(),
				latency:          msg.latency,
				latencyAvailable: msg.latencyAvailable,
				latencyHistory:   append([]opsLatencySample(nil), s.opsLatency...),
			}
		} else if s.opsStatus != nil {
			s.opsStatus.latencyAvailable = false
			s.opsStatus.latencyHistory = append([]opsLatencySample(nil), s.opsLatency...)
		}
		return s, nil
	case opsRefreshMsg:
		if !s.opsActive || msg.generation != s.opsGeneration {
			return s, nil
		}
		if !s.opsLoading {
			return s, tea.Batch(s.probeOps(msg.generation), s.scheduleOpsRefresh(msg.generation))
		}
		cmd := s.scheduleOpsRefresh(msg.generation)
		return s, cmd
	case hubFlowFailedMsg:
		s.opening = ""
		err := msg.err
		return s, func() tea.Msg { return wizard.ErrorSetMsg{Error: err} }
	case tea.KeyPressMsg:
		if msg.Code == tea.KeyEnter || msg.Code == tea.KeySpace {
			cmd := s.confirm()
			return s, cmd
		}
		if i, ok := s.acceleratedEntry(msg.Text); ok {
			s.nav.Select(i)
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
	if verb == HubVerbResumeDraft {
		msg := s.draftCursor
		return func() tea.Msg { return msg }
	}
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

// acceleratedEntry maps a pressed key to its menu entry: a verb's letter
// accelerator, or the 1-based digit of an accelerator-carrying row — destroy
// answers to neither, per the friction ladder.
func (s *WelcomeStep) acceleratedEntry(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	for i, e := range s.entries {
		if e.accel == "" {
			continue
		}
		if text == e.accel || text == strconv.Itoa(i+1) {
			return i, true
		}
	}
	return 0, false
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
	} else if s.opsActive {
		s.opsActive = false
		s.opsGeneration++
		s.opsLoading = false
		if s.opsCancel != nil {
			s.opsCancel()
			s.opsCancel = nil
			s.opsCtx = nil
		}
	}
}

// IsCentered keeps the launcher centered and gives a live dashboard the full viewport.
func (s *WelcomeStep) IsCentered() bool {
	return s.opsSource == nil
}

// RendersHero returns true so the hub owns its screen identity and the frame
// gives the launcher or operations dashboard the reclaimed header rows.
func (s *WelcomeStep) RendersHero() bool {
	return true
}

// SuppressesSplit gives the hub's launcher or dashboard the frame's full width.
func (s *WelcomeStep) SuppressesSplit() bool {
	return true
}

// SetTerminalSize records the terminal's own dimensions, which gate the hero's
// double-scale rendering.
func (s *WelcomeStep) SetTerminalSize(width, height int) {
	s.termWidth, s.termHeight = width, height
}

// View renders the blank-slate launcher or the existing-configuration hub.
func (s *WelcomeStep) View(width, height int) string {
	s.SetSize(width, height)
	if s.opsSource != nil {
		compact := width < 112 || s.termHeight < 30
		parts := []string{}
		if compact {
			if s.saveSlot != "" {
				parts = append(parts, tui.MutedStyle.Render(s.saveSlot))
			}
			parts = append(parts,
				renderOpsDashboard(s.opsStatus, s.opsLoading, s.opsErr, width, s.termHeight),
				"ACTIONS · ↑↓ choose · enter open",
				s.nav.ViewPointer(),
			)
		} else {
			if s.saveSlot != "" {
				parts = append(parts, tui.MutedStyle.Render(s.saveSlot), "")
			}
			parts = append(parts,
				renderOpsDashboard(s.opsStatus, s.opsLoading, s.opsErr, width, s.termHeight),
				"",
				tui.Card("HUB ACTIONS", renderOpsActionRows(s, width, s.termHeight)+"\n↑↓ choose · enter open", width, tui.ColorPrimary()),
			)
		}
		if s.opening != "" {
			parts = append(parts, "", wizard.Spinner(s.frame)+" "+tui.MutedStyle.Render("opening "+s.opening+"…"))
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	parts := []string{renderHero(s.termWidth, s.termHeight, tui.ColorEnabled()), ""}
	if s.saveSlot != "" {
		parts = append(parts, tui.MutedStyle.Render(s.saveSlot), "")
	}
	parts = append(parts, s.nav.ViewPointer())
	if s.opening != "" {
		parts = append(parts, "", wizard.Spinner(s.frame)+" "+tui.MutedStyle.Render("opening "+s.opening+"…"))
	}
	launcher := lipgloss.JoinVertical(lipgloss.Center, parts...)

	// The "get started" panel only describes what's ahead for a truly
	// blank slate (s.saveSlot == ""); a config already on disk reaches
	// this same centered layout too (no live cluster to probe yet), where
	// that copy would be wrong.
	if width < hubGetStartedPanelMinWidth || s.saveSlot != "" {
		return launcher
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, launcher, strings.Repeat(" ", hubGetStartedPanelGap), renderGetStartedPanel())
}

// hubGetStartedPanelMinWidth is the content width at and above which the
// blank-slate hub earns a second column instead of sitting as a lone
// centered menu in an otherwise empty frame — chosen so a 150-column
// terminal (the frame's own form+pane split threshold, wizard.SplitsFrame)
// clears it once the outer frame's own padding and border are subtracted.
const hubGetStartedPanelMinWidth = 140

const (
	hubGetStartedPanelGap   = 4
	hubGetStartedPanelWidth = 42
)

// renderGetStartedPanel renders the blank-slate hub's second column: an
// honest preview of the four phases "get started" walks through, drawn
// from the same connect/cluster/extras/review grouping the header trail
// and deployPhases use, not invented marketing copy.
func renderGetStartedPanel() string {
	phases := []struct{ label, detail string }{
		{"connect", "provider, distribution, credentials"},
		{labelCluster, "name, nodes, networking, resources"},
		{"extras", "add-ons, files, advanced settings"},
		{"review", "confirm changes, deploy"},
	}
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true)
	detailStyle := lipgloss.NewStyle().Foreground(tui.ColorTextDim())

	rows := make([]string, 0, len(phases)*2+2)
	for _, phase := range phases {
		rows = append(rows, labelStyle.Render(phase.label), detailStyle.Render("  "+phase.detail))
	}
	rows = append(rows, "", tui.DimStyle.Render("writes okdctl.yaml, ready to deploy"))
	return tui.Card("GET STARTED", strings.Join(rows, "\n"), hubGetStartedPanelWidth, tui.ColorAccent())
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
	case HubVerbEditConfig, HubVerbGetStarted, HubVerbResumeDraft:
		return false
	default:
		return true
	}
}
