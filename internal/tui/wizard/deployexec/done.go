package deployexec

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// resumeHint is the fix line the error card carries for every failed or
// cancelled deploy; the resume marker records the phase re-running picks up at.
const resumeHint = "re-run 'okdctl deploy' to resume from the recorded phase, or 'okdctl deploy --fresh' to restart from scratch"

// DoneStep is the terminal screen: the wizard-native rendering of the CLI
// post-deploy summary box, or the error card with the resume hint.
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

// InterceptBack keeps the flow forward-only: esc from the done screen would
// re-enter the finished stream step and softlock on its drained event channel.
func (s *DoneStep) InterceptBack() bool {
	return true
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

// View renders the CLI post-deploy summary for a success, or the CLI error box
// for a failed or cancelled run, sized to fit the wizard frame's inner width.
func (s *DoneStep) View(width, _ int) string {
	w := min(width, tui.DefaultBoxWidth)
	if s.st.Result != nil {
		return strings.Trim(render.ErrorCard(s.failureKind(), s.st.Result.Error(), resumeHint, w), "\n")
	}
	if s.st.Cfg == nil {
		return tui.CompletionSuccess("deployment complete")
	}
	return strings.Trim(render.PostDeploySummaryWidth(s.st.Cfg, s.st.Summary, s.st.Steps, s.st.RunID, w), "\n")
}

// failureKind names the outcome the error card leads with: a cancelled run was
// interrupted on purpose, anything else failed.
func (s *DoneStep) failureKind() string {
	if errors.Is(s.st.Result, context.Canceled) {
		return "deploy interrupted"
	}
	return "deploy failed"
}

// ShortHelp returns the completion help bar.
func (s *DoneStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: wizard.HelpEnter, Help: "exit"},
	}
}
