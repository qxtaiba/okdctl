package lifecycle

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// DoneStep is the terminal screen: the wizard-native rendering of the CLI
// completion box (outcome, per-node verbs, next steps), or the failure
// summary with the resume hint.
type DoneStep struct {
	wizard.BaseStep
	st *State
}

// NewDoneStep constructs the completion step.
func NewDoneStep(st *State) *DoneStep {
	return &DoneStep{
		BaseStep: wizard.NewBaseStep(StepIDDone, "done", ""),
		st:       st,
	}
}

// InterceptBack keeps the flow forward-only: esc from the done screen
// would re-enter the finished execution step and softlock on its drained
// event channel.
func (s *DoneStep) InterceptBack() bool {
	return true
}

// ShouldShow gates the step to consented (executed) runs.
func (s *DoneStep) ShouldShow(_ *config.Config) bool {
	return s.st.Proceed
}

// Init returns nil; the step only renders the recorded outcome.
func (s *DoneStep) Init() tea.Cmd {
	return nil
}

// Update completes the wizard on enter.
func (s *DoneStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.Code == tea.KeyEnter {
		return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDDone} }
	}
	return s, nil
}

// View renders the CLI completion box for a success, or the CLI error box
// for a failure, sized to fit the wizard frame's inner width.
func (s *DoneStep) View(width, height int) string {
	s.SetSize(width, height)
	w := min(width-2, tui.DefaultBoxWidth)
	if s.st.Result != nil {
		return strings.Trim(render.ErrorCard(string(s.st.Op)+" failed", s.st.Result.Error(),
			"re-run the same operation to resume at the recorded step", w), "\n")
	}
	if s.st.Plan == nil {
		return tui.CompletionSuccess("operation complete")
	}
	return strings.Trim(render.NodeOpCompleteWidth(s.st.Plan, s.st.Elapsed, w), "\n")
}

// ShortHelp returns the completion help bar.
func (s *DoneStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: wizard.HelpEnter, Help: "exit"},
	}
}
