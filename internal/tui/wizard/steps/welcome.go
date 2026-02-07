// Package steps provides the individual wizard step implementations.
package steps

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
)

// ═══════════════════════════════════════════════════════════════════════════════
// WELCOME STEP
// ═══════════════════════════════════════════════════════════════════════════════

// WelcomeMode indicates what user wants to do.
type WelcomeMode int

const (
	WelcomeModeDeploy WelcomeMode = iota
	WelcomeModeEdit
	WelcomeModeFresh
)

// WelcomeStep is the introductory step of the wizard.
type WelcomeStep struct {
	wizard.BaseStep
	configExists  bool
	selectedIndex int
	mode          WelcomeMode
}

// NewWelcomeStep creates a new welcome step.
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

// SetConfigExists tells the welcome step whether a config file exists.
func (s *WelcomeStep) SetConfigExists(exists bool) {
	s.configExists = exists
	if exists {
		s.selectedIndex = 0
		s.mode = WelcomeModeDeploy
	}
}

// GetMode returns the selected mode after step completes.
func (s *WelcomeStep) GetMode() WelcomeMode {
	return s.mode
}

// Init initializes the step.
func (s *WelcomeStep) Init() tea.Cmd {
	return nil
}

// Update handles input.
func (s *WelcomeStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if s.configExists && s.selectedIndex > 0 {
				s.selectedIndex--
				s.mode = WelcomeMode(s.selectedIndex)
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if s.configExists && s.selectedIndex < 2 {
				s.selectedIndex++
				s.mode = WelcomeMode(s.selectedIndex)
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", " "))):
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: s.ID()} }
		}
	}
	return s, nil
}

// IsCentered returns true so the wizard model vertically centers this step.
func (s *WelcomeStep) IsCentered() bool {
	return true
}

// View renders the welcome screen.
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
		content = titleStyle.Render("openshitctl setup") + "\n\n"
		content += subtitleStyle.Render("existing configuration detected") + "\n\n"

		options := []struct {
			title string
			desc  string
		}{
			{"deploy now", "deploy using current openshitctl.yaml"},
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
		content = titleStyle.Render("openshitctl setup") + "\n\n"
		content += subtitleStyle.Render("configure your kubernetes cluster") + "\n\n"
		content += s.renderOption("get started", "create your first configuration", true)
	}

	return content
}

// renderOption renders a selectable option.
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

// Apply resets config to defaults if "Start Fresh" was selected.
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

// GetSelectedAction returns the action for the selected mode.
func (s *WelcomeStep) GetSelectedAction() wizard.Action {
	if s.mode == WelcomeModeDeploy {
		return wizard.ActionDeploy
	}
	return ""
}

// ShouldExitEarly returns true if the wizard should exit after this step.
func (s *WelcomeStep) ShouldExitEarly() bool {
	return s.mode == WelcomeModeDeploy
}
