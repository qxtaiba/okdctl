package steps

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// HubVerb names the hub menu entry the operator picked.
type HubVerb int

// The hub's verbs: edit config and quit over an existing configuration, get
// started and quit on a blank slate. The others return with the commands
// that back them.
const (
	HubVerbDeploy HubVerb = iota
	HubVerbEditConfig
	HubVerbManageNodes
	HubVerbClusterStatus
	HubVerbDestroy
	HubVerbGetStarted
	HubVerbQuit
)

// hubEntry pairs a menu label with its verb; accel is the single-key
// accelerator in review's [N] grammar, empty for pointer-only rows.
type hubEntry struct {
	verb  HubVerb
	label string
	accel string
}

var hubVerbs = []hubEntry{
	{HubVerbEditConfig, "edit config", "e"},
	{HubVerbQuit, "quit", ""},
}

var hubFreshVerbs = []hubEntry{
	{HubVerbGetStarted, "get started", ""},
	{HubVerbQuit, "quit", ""},
}

// WelcomeStep is okdctl's hub.
type WelcomeStep struct {
	wizard.BaseStep
	configExists bool
	saveSlot     string
	entries      []hubEntry
	nav          *components.CompactSelector

	// termWidth and termHeight gate the hero's double scale in terminal columns
	termWidth  int
	termHeight int
}

// NewWelcomeStep constructs the hub on its blank-slate menu.
func NewWelcomeStep() *WelcomeStep {
	s := &WelcomeStep{
		BaseStep: wizard.NewBaseStep(wizard.StepIDWelcome, "welcome", ""),
	}
	s.setEntries(hubFreshVerbs)
	return s
}

// setEntries rebuilds the menu, indenting rows without an accelerator so the
// verbs share one column.
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

// SetConfigExists switches between the edit and blank-slate menus.
func (s *WelcomeStep) SetConfigExists(exists bool) {
	s.configExists = exists
	entries := hubFreshVerbs
	if exists {
		entries = hubVerbs
	} else {
		// A slot line left from an earlier SetExistingConfig would name a
		// configuration that is gone.
		s.saveSlot = ""
	}
	s.setEntries(entries)
}

// SetExistingConfig switches to the edit menu and derives the save-slot line
// from cfg.
func (s *WelcomeStep) SetExistingConfig(cfg *config.Config) {
	s.SetConfigExists(true)
	if cfg == nil {
		return
	}
	nodes := config.MasterCount + cfg.Workers.Count
	s.saveSlot = fmt.Sprintf("%s · okd %s · %s · configured",
		cfg.Cluster.Name, cfg.Distribution.Version, pluralNodes(nodes))
}

// SuppressesBadge hides the version badge on the blank-slate hub, where the
// defaults-seed version would advertise a cluster that does not exist.
func (s *WelcomeStep) SuppressesBadge() bool {
	return !s.configExists
}

func pluralNodes(n int) string {
	if n == 1 {
		return "1 node"
	}
	return fmt.Sprintf("%d nodes", n)
}

// SelectedVerb returns the highlighted verb.
func (s *WelcomeStep) SelectedVerb() HubVerb {
	i := s.nav.SelectedIndex()
	if i < 0 || i >= len(s.entries) {
		return HubVerbQuit
	}
	return s.entries[i].verb
}

// Init returns nil.
func (s *WelcomeStep) Init() tea.Cmd {
	return nil
}

// Update moves the pointer and confirms on enter, space or an accelerator.
func (s *WelcomeStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
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
	}
	return s, nil
}

func (s *WelcomeStep) confirm() tea.Cmd {
	return func() tea.Msg { return wizard.StepCompleteMsg{StepID: wizard.StepIDWelcome} }
}

// acceleratedEntry maps a key to its row by letter accelerator or the
// 1-based digit of an accelerator-carrying row.
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

// IsCentered keeps the launcher centered in the frame.
func (s *WelcomeStep) IsCentered() bool {
	return true
}

// RendersHero returns true so the frame drops its header for the launcher.
func (s *WelcomeStep) RendersHero() bool {
	return true
}

// OwnsFrameWidth gives the launcher the frame's full width.
func (s *WelcomeStep) OwnsFrameWidth() bool {
	return true
}

// SetTerminalSize records the terminal dimensions that gate the hero scale.
func (s *WelcomeStep) SetTerminalSize(width, height int) {
	s.termWidth, s.termHeight = width, height
}

// View renders the launcher with the save-slot line when a config exists.
func (s *WelcomeStep) View(width, height int) string {
	s.SetSize(width, height)
	parts := []string{renderHero(s.termWidth, s.termHeight, tui.ColorEnabled()), ""}
	if s.saveSlot != "" {
		parts = append(parts, tui.MutedStyle.Render(s.saveSlot), "")
	}
	parts = append(parts, s.nav.ViewPointer())
	launcher := lipgloss.JoinVertical(lipgloss.Center, parts...)

	// The "get started" panel describes a blank slate only.
	if width < hubGetStartedPanelMinWidth || s.saveSlot != "" {
		return launcher
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, launcher, strings.Repeat(" ", hubGetStartedPanelGap), renderGetStartedPanel())
}

// hubGetStartedPanelMinWidth lets a 150-column terminal clear it after the
// frame's own padding and border.
const hubGetStartedPanelMinWidth = 140

const (
	hubGetStartedPanelGap   = 4
	hubGetStartedPanelWidth = 42
)

// renderGetStartedPanel previews the four phases the header trail groups.
func renderGetStartedPanel() string {
	phases := []struct{ label, detail string }{
		{"connect", "provider, distribution, credentials"},
		{labelCluster, "name, nodes, networking, resources"},
		{"extras", "add-ons, files, advanced settings"},
		{"review", "confirm changes, save"},
	}
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true)
	detailStyle := lipgloss.NewStyle().Foreground(tui.ColorTextDim())

	rows := make([]string, 0, len(phases)*2+2)
	for _, phase := range phases {
		rows = append(rows, labelStyle.Render(phase.label), detailStyle.Render("  "+phase.detail))
	}
	rows = append(rows, "", tui.DimStyle.Render("writes okdctl.yaml"))
	return tui.Card("GET STARTED", strings.Join(rows, "\n"), hubGetStartedPanelWidth, tui.ColorAccent())
}

// Validate always returns nil.
func (s *WelcomeStep) Validate() error {
	return nil
}

// ShortHelp labels enter "start": the hub begins the wizard rather than
// advancing a form.
func (s *WelcomeStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "choose"},
		{Key: wizard.HelpEnter, Help: wizard.HelpStart},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}

// ShouldExitEarly reports whether the highlighted verb leaves the wizard
// instead of walking the configure flow.
func (s *WelcomeStep) ShouldExitEarly() bool {
	switch s.SelectedVerb() {
	case HubVerbEditConfig, HubVerbGetStarted:
		return false
	default:
		return true
	}
}
