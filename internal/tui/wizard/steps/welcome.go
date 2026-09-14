package steps

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// WelcomeMode selects the welcome-step flow chosen by the user.
type WelcomeMode int

// Welcome mode values, index-aligned with welcomeOptions.
const (
	WelcomeModeDeploy WelcomeMode = iota
	WelcomeModeEdit
	WelcomeModeFresh
)

var welcomeOptions = []struct {
	title string
	desc  string
}{
	{"deploy now", "deploy using current okdctl.yaml"},
	{"edit existing", "modify your current settings"},
	{"start fresh", "create a new configuration"},
}

// welcomeNeedItems lists the "you'll need" checklist rendered beside the hero.
var welcomeNeedItems = []string{
	"a proxmox host",
	"proxmox credentials",
	"an okd pull secret",
}

// welcomeTakesItems lists the "it takes" timing estimates rendered beside the hero.
var welcomeTakesItems = []string{
	"≈ 10 min to configure",
	"≈ 45 min to deploy",
}

var (
	welcomeTaglineStyle     = lipgloss.NewStyle().Foreground(tui.ColorSlate400).Italic(true)
	welcomeColumnTitleStyle = tui.TextStyle.Bold(true)
)

// WelcomeStep is the wizard's entry screen, offering deploy/edit/fresh options
// when a config already exists.
type WelcomeStep struct {
	wizard.BaseStep
	configExists bool
	foundLine    string
	nav          *components.CompactSelector
}

// NewWelcomeStep constructs the welcome wizard step.
func NewWelcomeStep() *WelcomeStep {
	return &WelcomeStep{
		BaseStep: wizard.NewBaseStep(
			wizard.StepIDWelcome,
			"welcome",
			"",
		),
		nav: newWelcomeSelect(false),
	}
}

// newWelcomeSelect builds a clamped (non-wrapping) selector over
// deploy/edit/fresh, or a single "get started" entry.
func newWelcomeSelect(configExists bool) *components.CompactSelector {
	options := []string{"get started"}
	if configExists {
		options = make([]string, len(welcomeOptions))
		for i, opt := range welcomeOptions {
			options[i] = opt.title
		}
	}
	selector := components.NewCompactSelector(options)
	selector.SetWrap(false)
	return selector
}

// SetConfigExists tells the step whether okdctl.yaml exists, switching between
// the deploy/edit/fresh and blank onboarding branches.
func (s *WelcomeStep) SetConfigExists(exists bool) {
	s.configExists = exists
	if exists {
		s.nav = newWelcomeSelect(true)
	}
}

// SetExistingConfig marks an existing okdctl.yaml as present and derives the
// found line's cluster name and node counts from cfg.
func (s *WelcomeStep) SetExistingConfig(cfg *config.Config) {
	s.SetConfigExists(true)
	if cfg == nil {
		return
	}
	s.foundLine = fmt.Sprintf("found okdctl.yaml · cluster %s · %d + %d nodes",
		cfg.Cluster.Name, cfg.Topology.ControlPlane.Count, cfg.Topology.Workers.Count)
}

// GetMode returns the currently-selected welcome mode.
func (s *WelcomeStep) GetMode() WelcomeMode {
	if !s.configExists {
		return WelcomeModeFresh
	}
	return WelcomeMode(s.nav.SelectedIndex())
}

// Init returns nil; the step has no async startup work.
func (s *WelcomeStep) Init() tea.Cmd {
	return nil
}

// Update handles left/right/up/down navigation (arrows remapped onto the
// selector's vertical bindings) and enter/space to advance.
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

// IsCentered returns true so the welcome screen is rendered centered.
func (s *WelcomeStep) IsCentered() bool {
	return true
}

// View renders the block-letter hero, the you'll-need/it-takes checklist
// columns, an optional found-config line, and the horizontal mode radio.
func (s *WelcomeStep) View(width, height int) string {
	s.SetSize(width, height)

	hero := renderHero(width, tui.ColorEnabled())
	tagline := welcomeTaglineStyle.Render("answer a few questions, then deploy")
	columns := lipgloss.JoinHorizontal(lipgloss.Top, s.renderNeedList(), "    ", s.renderTakesList())

	parts := []string{hero, tagline, columns, ""}
	if s.foundLine != "" {
		parts = append(parts, tui.MutedStyle.Render(s.foundLine))
	}
	parts = append(parts, s.nav.ViewInline(), tui.MutedStyle.Render(s.selectedDesc()))

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

// renderNeedList renders the "you'll need" checklist column.
func (s *WelcomeStep) renderNeedList() string {
	lines := make([]string, 0, len(welcomeNeedItems)+1)
	lines = append(lines, welcomeColumnTitleStyle.Render("you'll need"))
	for _, item := range welcomeNeedItems {
		lines = append(lines, tui.SuccessStyle.Render(tui.IconSuccess)+" "+tui.TextStyle.Render(item))
	}
	return strings.Join(lines, "\n")
}

// renderTakesList renders the "it takes" timing-estimate column.
func (s *WelcomeStep) renderTakesList() string {
	lines := make([]string, 0, len(welcomeTakesItems)+1)
	lines = append(lines, welcomeColumnTitleStyle.Render("it takes"))
	for _, item := range welcomeTakesItems {
		lines = append(lines, tui.TextStyle.Render(item))
	}
	return strings.Join(lines, "\n")
}

// selectedDesc returns the description shown dim beneath the radio for the currently-selected option.
func (s *WelcomeStep) selectedDesc() string {
	if !s.configExists {
		return "create your first configuration"
	}
	return welcomeOptions[s.nav.SelectedIndex()].desc
}

// Validate always returns nil; the welcome step has no inputs to validate.
func (s *WelcomeStep) Validate() error {
	return nil
}

// Apply resets cfg to package defaults when the user picked WelcomeModeFresh
// over an existing configuration.
func (s *WelcomeStep) Apply(cfg *config.Config) error {
	if s.GetMode() == WelcomeModeFresh && s.configExists {
		freshCfg := config.DefaultConfig()
		*cfg = *freshCfg
	}
	return nil
}

// ShortHelp returns the help bar shown on the welcome screen.
func (s *WelcomeStep) ShortHelp() []wizard.KeyBinding {
	var bindings []wizard.KeyBinding
	if s.configExists {
		bindings = append(bindings, wizard.KeyBinding{Key: "←/→", Help: "choose"})
	}
	bindings = append(bindings,
		wizard.KeyBinding{Key: wizard.HelpEnter, Help: "start"},
		wizard.KeyBinding{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	)
	return bindings
}

// GetSelectedAction maps the chosen welcome mode to a wizard.Action.
func (s *WelcomeStep) GetSelectedAction() wizard.Action {
	if s.GetMode() == WelcomeModeDeploy {
		return wizard.ActionDeploy
	}
	return wizard.ActionExit
}

// ShouldExitEarly reports whether the user chose deploy-now, skipping the rest of the wizard.
func (s *WelcomeStep) ShouldExitEarly() bool {
	return s.GetMode() == WelcomeModeDeploy
}
