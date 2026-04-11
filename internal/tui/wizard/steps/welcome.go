package steps

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type WelcomeMode int

const (
	WelcomeModeDeploy WelcomeMode = iota
	WelcomeModeEdit
	WelcomeModeFresh
)

// welcomeModeCount is the number of WelcomeMode values. Used to bound the
// selectedIndex in the welcome step's up/down navigation.
const welcomeModeCount = int(WelcomeModeFresh) + 1

type WelcomeStep struct {
	wizard.BaseStep
	configExists  bool
	selectedIndex int
	mode          WelcomeMode
}

func NewWelcomeStep() *WelcomeStep {
	return &WelcomeStep{
		BaseStep: wizard.NewBaseStep(
			wizard.StepIDWelcome,
			"welcome",
			"",
		),
		selectedIndex: 0,
		mode:          WelcomeModeFresh,
	}
}

func (s *WelcomeStep) SetConfigExists(exists bool) {
	s.configExists = exists
	if exists {
		s.selectedIndex = 0
		s.mode = WelcomeModeDeploy
	}
}

func (s *WelcomeStep) GetMode() WelcomeMode {
	return s.mode
}

func (s *WelcomeStep) Init() tea.Cmd {
	return nil
}

func (s *WelcomeStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "k"))):
			if s.configExists && s.selectedIndex > 0 {
				s.selectedIndex--
				s.mode = WelcomeMode(s.selectedIndex)
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "j"))):
			if s.configExists && s.selectedIndex < welcomeModeCount-1 {
				s.selectedIndex++
				s.mode = WelcomeMode(s.selectedIndex)
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter", " "))):
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: s.ID()} }
		}
	}
	return s, nil
}

func (s *WelcomeStep) IsCentered() bool {
	return true
}

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

		options := []struct {
			title string
			desc  string
		}{
			{"deploy now", "deploy using current okdctl.yaml"},
			{"edit existing", "modify your current settings"},
			{"start fresh", "create a new configuration"},
		}

		for i, opt := range options {
			content += s.renderOption(opt.title, opt.desc, i == s.selectedIndex)
			if i < len(options)-1 {
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

func (s *WelcomeStep) Validate() error {
	return nil
}

func (s *WelcomeStep) Apply(cfg *config.Config) error {
	if s.mode == WelcomeModeFresh && s.configExists {
		freshCfg := config.DefaultConfig()
		*cfg = *freshCfg
	}
	return nil
}

func (s *WelcomeStep) ShortHelp() []wizard.KeyBinding {
	if s.configExists {
		return []wizard.KeyBinding{
			{Key: "↑↓", Help: "select"},
			{Key: "enter", Help: "confirm"},
			{Key: "ctrl+c", Help: "quit"},
		}
	}
	return []wizard.KeyBinding{
		{Key: "enter", Help: "start"},
		{Key: "ctrl+c", Help: "quit"},
	}
}

func (s *WelcomeStep) GetSelectedAction() wizard.Action {
	if s.mode == WelcomeModeDeploy {
		return wizard.ActionDeploy
	}
	return wizard.ActionExit
}

func (s *WelcomeStep) ShouldExitEarly() bool {
	return s.mode == WelcomeModeDeploy
}
