package lifecycle

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

// ConfirmStep is the typed-name gate for destructive plans, mirroring the
// CLI's two-stage --confirm-cluster requirement: enter stays inert until
// the operator types the exact cluster name.
type ConfirmStep struct {
	wizard.BaseStep
	st    *State
	input *components.InputField
}

// NewConfirmStep constructs the typed-confirmation step.
func NewConfirmStep(st *State) *ConfirmStep {
	input := components.NewInputField("", "")
	return &ConfirmStep{
		BaseStep: wizard.NewBaseStep(StepIDConfirm, "confirm", ""),
		st:       st,
		input:    input,
	}
}

// ShouldShow gates the step to consented, VM-destroying plans only.
func (s *ConfirmStep) ShouldShow(_ *config.Config) bool {
	return s.st.Proceed && s.st.Plan != nil && s.st.Plan.DestroysData()
}

// Init clears any previously typed name so re-entry always re-confirms.
func (s *ConfirmStep) Init() tea.Cmd {
	s.input.SetValue("")
	return s.input.Focus()
}

// IsCentered returns true so the confirmation renders centered.
func (s *ConfirmStep) IsCentered() bool {
	return true
}

// Update forwards typing to the input; enter completes only on an exact
// cluster-name match.
func (s *ConfirmStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.Code == tea.KeyEnter {
		if strings.TrimSpace(s.input.Value()) == s.st.Cfg.Cluster.Name {
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDConfirm} }
		}
		return s, nil
	}
	var cmd tea.Cmd
	var field components.FormField
	field, cmd = s.input.Update(msg)
	if in, ok := field.(*components.InputField); ok {
		s.input = in
	}
	return s, cmd
}

// View renders the irreversible warning and the typed-name prompt.
func (s *ConfirmStep) View(width, height int) string {
	s.SetSize(width, height)

	titleStyle := lipgloss.NewStyle().Foreground(tui.ColorError).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
	promptStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate300)

	names := make([]string, 0, len(s.st.Plan.Nodes))
	for i := range s.st.Plan.Nodes {
		names = append(names, s.st.Plan.Nodes[i].Name)
	}

	s.input.SetWidth(min(width-8, 52))

	var b strings.Builder
	b.WriteString(titleStyle.Render("confirm irreversible removal"))
	b.WriteString("\n\n")
	b.WriteString(warnStyle.Render(fmt.Sprintf("irreversible: destroys %s and its data disk;", strings.Join(names, ", "))))
	b.WriteString("\n")
	b.WriteString(warnStyle.Render("removed data cannot be recovered"))
	b.WriteString("\n\n")
	b.WriteString(promptStyle.Render(fmt.Sprintf("type the cluster name %q to confirm", s.st.Cfg.Cluster.Name)))
	b.WriteString("\n")
	b.WriteString(s.input.View())
	return b.String()
}

// SetFocused propagates focus to the input.
func (s *ConfirmStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	if focused {
		_ = s.input.Focus() // command executed during Init
		return
	}
	s.input.Blur()
}

// ShortHelp returns the confirmation help bar.
func (s *ConfirmStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: wizard.HelpEnter, Help: "confirm (disabled until name matches)"},
		{Key: wizard.HelpEsc, Help: wizard.HelpBack},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}
