package deployexec

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// resumeHint is the fix line the error card carries for every failed or
// cancelled deploy; the resume marker records the phase re-running picks up at.
const resumeHint = "re-run 'okdctl deploy' to resume from the recorded phase, or 'okdctl deploy --fresh' to restart from scratch"

// DoneStep is the terminal screen: the wizard-native rendering of the CLI
// post-deploy summary box, or the error card with the resume hint.
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

// InterceptBack keeps the flow forward-only: esc from the done screen would
// re-enter the finished stream step and softlock on its drained event channel.
func (s *DoneStep) InterceptBack() bool {
	return true
}

// Init returns nil; the step only renders the recorded outcome.
func (s *DoneStep) Init() tea.Cmd {
	return nil
}

// Update completes the wizard on enter; on a failure, pgup/pgdn page the log
// region back through the ring — the evidence must be diggable from here, not
// only from a full-screen mode entered before the run died.
func (s *DoneStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	switch keyMsg.Code {
	case tea.KeyEnter:
		return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDDone} }
	case tea.KeyPgUp:
		s.scrollLogBy(-s.logPageSize())
	case tea.KeyPgDown:
		s.scrollLogBy(s.logPageSize())
	}
	return s, nil
}

// ConsumesPaging hands pgup/pgdn to the failure screen's log region; a
// success keeps the frame's viewport paging — the summary is the record
// there, and it may itself overflow.
func (s *DoneStep) ConsumesPaging() bool {
	return s.hooks.Logs != nil && s.st.Result != nil
}

// ScrollsWithArrows opts the read-only completion screen into the frame's
// line-by-line arrow scroll.
func (s *DoneStep) ScrollsWithArrows() bool {
	return true
}

// scrollLogBy moves the log window n lines through the ring at the geometry
// last rendered: the split pane, or the failure tail under the error card.
func (s *DoneStep) scrollLogBy(n int) {
	s.log.ScrollBy(n, s.splitsFrame())
}

// logPageSize is how many lines one pgup/pgdn moves: the lines the log
// region is showing.
func (s *DoneStep) logPageSize() int {
	return s.log.PageSize(s.splitsFrame())
}

// View renders the CLI post-deploy summary for a success, or the CLI error box
// for a failed or cancelled run, sized to fit the wizard frame's inner width.
func (s *DoneStep) View(width, _ int) string {
	w := min(width, tui.DefaultBoxWidth)
	s.log.ViewCol = max(width-4, 1)
	if s.st.Result != nil {
		card := strings.Trim(render.ErrorCard(s.failureKind(), s.st.Result.Error(), resumeHint, w), "\n")
		return card + s.failureTail(s.log.ViewCol) + s.sinkLine(s.log.ViewCol)
	}
	if s.st.Cfg == nil {
		return tui.CompletionSuccess("deployment complete")
	}
	return strings.Trim(render.PostDeploySummaryWidth(s.st.Cfg, s.st.Summary, s.st.Steps, s.st.RunID, w), "\n")
}

// failureTail appends the log's last lines under the error card on a frame with
// no pane to carry them. A failure's evidence is the chatter that led up to it,
// so it stays on screen at every width; a success needs none — its summary box
// is the record.
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

// sinkLine names the run-log file under the failure evidence — the ring holds
// a window, the file keeps every byte — or nothing when no sink is open.
func (s *DoneStep) sinkLine(col int) string {
	line := logview.SinkLine(s.hooks.LogPath, col)
	if line == "" {
		return ""
	}
	return "\n\n" + line
}

// failureKind names the outcome the error card leads with: a cancelled run was
// interrupted on purpose, anything else failed.
func (s *DoneStep) failureKind() string {
	if errors.Is(s.st.Result, context.Canceled) {
		return "deploy interrupted"
	}
	return "deploy failed"
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
