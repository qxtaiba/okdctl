package lifecycle

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// DoneStep is the terminal screen: the wizard-native rendering of the CLI
// completion box (outcome, per-node verbs, next steps), or the failure
// summary with the resume hint and the run's evidence tail.
type DoneStep struct {
	wizard.BaseStep
	frameSize
	st    *State
	hooks Hooks
	log   logview.Surface
}

// NewDoneStep constructs the completion step.
func NewDoneStep(st *State, hooks Hooks) *DoneStep {
	return &DoneStep{
		BaseStep: wizard.NewBaseStep(StepIDDone, "done", ""),
		st:       st,
		hooks:    hooks,
		log:      logview.Surface{Src: hooks.Logs},
	}
}

// SetSize records the body box the frame gives the step, which the failure
// tail's own budget is measured against.
func (s *DoneStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	s.bodyHeight = height
}

// PaneContent keeps the live log in the split layout's right pane on the
// completion screen too: the run just ended, and its last lines are what an
// operator reads next.
func (s *DoneStep) PaneContent(width, height int) string {
	return s.log.RenderPane(width, height)
}

// InterceptBack keeps the flow forward-only: esc from the done screen
// would re-enter the finished execution step and softlock on its drained
// event channel.
func (s *DoneStep) InterceptBack() bool {
	return true
}

// ScrollsWithArrows opts the read-only completion screen into the frame's
// line-by-line arrow scroll.
func (s *DoneStep) ScrollsWithArrows() bool {
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

// Update completes the wizard on enter; on a failure, pgup/pgdn page the
// log region back through the ring — the evidence must be diggable from
// here, not only from a full-screen mode entered before the run died.
func (s *DoneStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	switch keyMsg.Code {
	case tea.KeyEnter:
		return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDDone} }
	case tea.KeyPgUp:
		s.log.ScrollBy(-s.log.PageSize(s.splitsFrame()), s.splitsFrame())
	case tea.KeyPgDown:
		s.log.ScrollBy(s.log.PageSize(s.splitsFrame()), s.splitsFrame())
	}
	return s, nil
}

// ConsumesPaging hands pgup/pgdn to the failure screen's log region; a
// success keeps the frame's viewport paging — the summary is the record
// there, and it may itself overflow.
func (s *DoneStep) ConsumesPaging() bool {
	return s.hooks.Logs != nil && s.st.Result != nil
}

// View renders the CLI completion box for a success, or the CLI error box
// with the run's evidence tail for a failure, sized to fit the wizard
// frame's inner width.
func (s *DoneStep) View(width, _ int) string {
	w := min(width, tui.DefaultBoxWidth)
	s.log.ViewCol = max(width-4, 1)
	if s.st.Result != nil {
		card := strings.Trim(render.ErrorCard(s.failureKind(), s.st.Result.Error(),
			"re-run the same operation to resume at the recorded step", w), "\n")
		return card + s.failureTail(s.log.ViewCol) + s.sinkLine(s.log.ViewCol)
	}
	if s.st.Plan == nil {
		return tui.CompletionSuccess("operation complete")
	}
	return strings.Trim(render.NodeOpCompleteWidth(s.st.Plan, s.st.Elapsed, w), "\n")
}

// failureTail appends the log's last lines under the error card on a frame
// with no pane to carry them. A failure's evidence is the chatter that led
// up to it; a success needs none — its completion box is the record.
func (s *DoneStep) failureTail(col int) string {
	if s.splitsFrame() {
		return ""
	}
	tail := s.log.FailureTail(col)
	if tail == "" {
		return ""
	}
	return "\n\n" + tail
}

// sinkLine names the run-log file under the failure evidence — the ring
// holds a window, the file keeps every byte — or nothing when no sink is open.
func (s *DoneStep) sinkLine(col int) string {
	line := logview.SinkLine(s.hooks.LogPath, col)
	if line == "" {
		return ""
	}
	return "\n\n" + line
}

// failureKind names the outcome the error card leads with: a cancelled run
// was interrupted on purpose, anything else failed — the same distinction
// deployexec's done screen draws.
func (s *DoneStep) failureKind() string {
	if errors.Is(s.st.Result, context.Canceled) {
		return string(s.st.Op) + " interrupted"
	}
	return string(s.st.Op) + " failed"
}

// ShortHelp returns the completion help bar, advertising the failure
// screen's log paging where it is live.
func (s *DoneStep) ShortHelp() []wizard.KeyBinding {
	bindings := []wizard.KeyBinding{
		{Key: wizard.HelpEnter, Help: "exit"},
	}
	if s.ConsumesPaging() {
		bindings = append(bindings, wizard.KeyBinding{Key: "pgup/pgdn", Help: "page the log"})
	}
	return bindings
}
