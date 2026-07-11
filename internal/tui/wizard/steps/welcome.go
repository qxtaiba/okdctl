package steps

import (
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

// WelcomeStep is the wizard's entry screen, offering deploy/edit/fresh
// options when an existing config is present.
type WelcomeStep struct {
	wizard.BaseStep
	configExists bool
	nav          *singleSelect
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

// newWelcomeSelect builds the step's navigation: a clamped (non-wrapping)
// list over the deploy/edit/fresh options, or a single "get started" entry
// on the blank onboarding branch. Space confirms alongside enter.
func newWelcomeSelect(configExists bool) *singleSelect {
	options := []string{"get started"}
	if configExists {
		options = make([]string, len(welcomeOptions))
		for i, opt := range welcomeOptions {
			options[i] = opt.title
		}
	}
	selector := components.NewCompactSelector(options)
	selector.SetWrap(false)
	return newSingleSelect(wizard.StepIDWelcome, selector, "enter", " ")
}

// SetConfigExists tells the step whether an okdctl.yaml exists so it can
// offer the deploy/edit/fresh branch instead of the blank onboarding branch.
func (s *WelcomeStep) SetConfigExists(exists bool) {
	s.configExists = exists
	if exists {
		s.nav = newWelcomeSelect(true)
	}
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

// Update handles up/down navigation and enter to advance.
func (s *WelcomeStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	return s, s.nav.Update(msg)
}

// IsCentered returns true so the welcome screen is rendered centered.
func (s *WelcomeStep) IsCentered() bool {
	return true
}

// View renders the welcome screen with the appropriate option set.
func (s *WelcomeStep) View(width, height int) string {
	s.SetSize(width, height)

	titleStyle := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(tui.ColorSlate400).
		Italic(true)

	var content string

	if s.configExists {
		content = titleStyle.Render("okdctl setup") + "\n\n"
		content += subtitleStyle.Render("existing configuration detected") + "\n\n"

		for i, opt := range welcomeOptions {
			content += s.renderOption(opt.title, opt.desc, i == s.nav.SelectedIndex())
			if i < len(welcomeOptions)-1 {
				content += "\n\n"
			}
		}
	} else {
		content = titleStyle.Render("okdctl setup") + "\n\n"
		content += subtitleStyle.Render("configure your kubernetes cluster") + "\n\n"
		content += s.renderOption("get started", "create your first configuration", true)
	}

	return content
}

func (s *WelcomeStep) renderOption(title, description string, selected bool) string {
	var bullet, titleStyled string

	if selected {
		bullet = lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true).Render("●")
		titleStyled = lipgloss.NewStyle().Foreground(tui.ColorText).Bold(true).Render(title)
	} else {
		bullet = lipgloss.NewStyle().Foreground(tui.ColorSlate600).Render("○")
		titleStyled = lipgloss.NewStyle().Foreground(tui.ColorSlate300).Render(title)
	}

	descStyled := lipgloss.NewStyle().Foreground(tui.ColorSlate500).Render(description)

	return bullet + " " + titleStyled + "\n  " + descStyled
}

// Validate always returns nil; the welcome step has no inputs to validate.
func (s *WelcomeStep) Validate() error {
	return nil
}

// Apply resets cfg to the package defaults when the user picked
// WelcomeModeFresh on top of an existing configuration.
func (s *WelcomeStep) Apply(cfg *config.Config) error {
	if s.GetMode() == WelcomeModeFresh && s.configExists {
		freshCfg := config.DefaultConfig()
		*cfg = *freshCfg
	}
	return nil
}

// ShortHelp returns the help bar shown on the welcome screen.
func (s *WelcomeStep) ShortHelp() []wizard.KeyBinding {
	if s.configExists {
		return []wizard.KeyBinding{
			{Key: "↑↓", Help: "select"},
			{Key: helpEnter, Help: helpConfirm},
			{Key: helpCtrlC, Help: helpQuit},
		}
	}
	return []wizard.KeyBinding{
		{Key: helpEnter, Help: "start"},
		{Key: helpCtrlC, Help: helpQuit},
	}
}

// GetSelectedAction maps the chosen welcome mode to a wizard.Action.
func (s *WelcomeStep) GetSelectedAction() wizard.Action {
	if s.GetMode() == WelcomeModeDeploy {
		return wizard.ActionDeploy
	}
	return wizard.ActionExit
}

// ShouldExitEarly reports whether the user chose deploy-now, which skips
// the rest of the wizard steps.
func (s *WelcomeStep) ShouldExitEarly() bool {
	return s.GetMode() == WelcomeModeDeploy
}
