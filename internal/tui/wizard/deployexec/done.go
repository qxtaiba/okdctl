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

// The keys the failure report answers to: the surface's full-screen log, and
// the one non-destructive action worth a keystroke.
const (
	keyFullLog   = logview.KeyFull
	keyCopyRunID = 'c'
)

// DoneStep is the terminal screen: the wizard-native rendering of the CLI
// post-deploy summary box, or the failure incident report — the frozen phase
// checklist, the error card, the run's identity, the next moves, and the
// evidence that led up to it.
type DoneStep struct {
	wizard.BaseStep
	wizard.FrameSize
	wizard.ExecStyleCache
	st    *State
	hooks Hooks
	log   logview.Surface
	// copied records that the run id was written to the clipboard, so the
	// report can say the escape went out without claiming the terminal took it.
	copied bool
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
	s.SetBodyHeight(height)
}

// PaneContent keeps the live log in the split layout's right pane on the
// completion screen too: the run just ended, and its last lines are what an
// operator reads next.
func (s *DoneStep) PaneContent(width, height int) string {
	return s.log.RenderPane(width, height)
}

// InterceptBack keeps the flow forward-only: esc from the done screen would
// re-enter the finished stream step and softlock on its drained event channel.
// Closing an open filter input is the one thing the key does here — the frame
// consumes esc before the step's own Update could see it.
func (s *DoneStep) InterceptBack() bool {
	s.log.CancelFilter()
	return true
}

// Init returns nil; the step only renders the recorded outcome.
func (s *DoneStep) Init() tea.Cmd {
	return nil
}

// Update completes the wizard on enter and, on a failure, hands every other
// key to the log surface: the evidence must be pageable, filterable and
// jumpable from here, not only from a full-screen mode entered before the run
// died. Enter belongs to an open filter input first — committing a pattern
// must never quit the wizard instead.
func (s *DoneStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	if s.st.Result == nil {
		if keyMsg.Code == tea.KeyEnter {
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDDone} }
		}
		return s, nil
	}
	if keyMsg.Code == tea.KeyEnter && !s.log.Filtering() {
		return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDDone} }
	}
	if keyMsg.Text == string(rune(keyCopyRunID)) && !s.log.Filtering() {
		cmd := s.copyRunID()
		return s, cmd
	}
	if s.log.HandleKey(keyMsg, s.SplitsFrame(flowStepCount)) {
		return s, func() tea.Msg { return wizard.LayoutChangedMsg{} }
	}
	return s, nil
}

// copyRunID writes the run id to the system clipboard over OSC 52, gated the
// way every escape emission is: a pipe or NO_COLOR run emits nothing and the
// report says nothing either.
func (s *DoneStep) copyRunID() tea.Cmd {
	if s.st.RunID == "" || !tui.ColorEnabled() {
		return nil
	}
	s.copied = true
	return tea.SetClipboard(s.st.RunID)
}

// ConsumesPaging hands pgup/pgdn to the failure screen's log region; a
// success keeps the frame's viewport paging — the summary is the record
// there, and it may itself overflow.
func (s *DoneStep) ConsumesPaging() bool {
	return s.hooks.Logs != nil && s.st.Result != nil
}

// ConsumesTextInput reports the failure report's open filter input, so the
// frame hands over every printable key instead of acting on it.
func (s *DoneStep) ConsumesTextInput() bool {
	return s.log.Filtering()
}

// ScrollsWithArrows opts the read-only completion screen into the frame's
// line-by-line arrow scroll; in full-screen log mode the arrows walk the log
// instead.
func (s *DoneStep) ScrollsWithArrows() bool {
	return !s.log.Full()
}

// SuppressesSplit hands the log the whole frame while the failure report has
// it full-screen.
func (s *DoneStep) SuppressesSplit() bool {
	return s.log.Full() && s.hooks.Logs != nil
}

// View renders the CLI post-deploy summary for a success, or the failure
// incident report for a failed or cancelled run, sized to fit the wizard
// frame's inner width.
func (s *DoneStep) View(width, _ int) string {
	w := min(width, tui.DefaultBoxWidth)
	col := max(width-4, 1)
	s.log.ViewCol = col
	if s.st.Result != nil {
		if s.log.Full() {
			return s.fullLog(col)
		}
		return s.incidentReport(w, col)
	}
	if s.st.Cfg == nil {
		return tui.CompletionSuccess("deployment complete")
	}
	return strings.Trim(render.PostDeploySummaryWidth(s.st.Cfg, s.st.Summary, s.st.Steps, s.st.RunID, w), "\n")
}

// incidentReport composes the failure screen from the parts the run already
// produced: the phase checklist frozen where it stopped, the error card, the
// run's identity as facts, the moves that follow, and the evidence tail that
// led up to it. The card carries no hint of its own — the next-moves block is
// the fix, stated once.
func (s *DoneStep) incidentReport(boxWidth, col int) string {
	card := strings.Trim(render.ErrorCard(s.failureKind(), s.st.Result.Error(), "", boxWidth), "\n")
	return section(
		frozenChecklist(s.st, s.Styles(), col),
		strings.Split(card, "\n"),
		incidentFacts(s.st, col),
		nextMoves(s.Styles(), col, s.copied),
		s.evidence(col),
		s.sinkLine(col),
	)
}

// fullLog renders the whole body as the log, the report's own "open the full
// log" move, with the sink path above it.
func (s *DoneStep) fullLog(col int) string {
	head := []string{s.Styles().Bold.Render("evidence · " + s.failureKind())}
	if s.hooks.LogPath != "" {
		head = append(head, s.Styles().Dim.Render(tui.Truncate("full log: "+s.hooks.LogPath, col)))
	}
	return strings.Join(head, "\n") + "\n" + s.log.RenderFull(col, max(s.BodyHeight()-len(head), 2))
}

// evidence renders the log's last lines under the report on a frame with no
// pane to carry them. A failure's evidence is the chatter that led up to it,
// so it stays on screen at every width; a success needs none — its summary box
// is the record.
func (s *DoneStep) evidence(col int) []string {
	if s.SplitsFrame(flowStepCount) {
		return nil
	}
	tail := s.log.FailureTail(col)
	if tail == "" {
		return nil
	}
	return strings.Split(tail, "\n")
}

// sinkLine names the run-log file under the failure evidence — the ring holds
// a window, the file keeps every byte — or nothing when no sink is open.
func (s *DoneStep) sinkLine(col int) []string {
	line := logview.SinkLine(s.hooks.LogPath, col)
	if line == "" {
		return nil
	}
	return strings.Split(line, "\n")
}

// failureKind names the outcome the error card leads with: a cancelled run was
// interrupted on purpose, anything else failed.
func (s *DoneStep) failureKind() string {
	if errors.Is(s.st.Result, context.Canceled) {
		return "deploy interrupted"
	}
	return "deploy failed"
}

// ShortHelp returns the completion help bar: a success exits, and a failure
// advertises the triage keys its evidence region answers to plus the one
// action worth a keystroke.
func (s *DoneStep) ShortHelp() []wizard.KeyBinding {
	exit := wizard.KeyBinding{Key: wizard.HelpEnter, Help: "exit"}
	if s.hooks.Logs == nil || s.st.Result == nil {
		return []wizard.KeyBinding{exit}
	}
	return wizard.LogHelp(&s.log,
		wizard.KeyBinding{Key: string(rune(keyCopyRunID)), Help: "copy the run id"},
		exit)
}
