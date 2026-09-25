package deployexec

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type rowStatus int

const (
	rowPending rowStatus = iota
	rowRunning
	rowDone
	rowSkipped
	rowFailed
)

// stepRow is one registered deploy step's line on the checklist.
type stepRow struct {
	id     distribution.StepID
	label  string
	status rowStatus
	start  time.Time
	took   time.Duration
}

// phaseProgress is one checklist group: its rows plus the span it occupied.
type phaseProgress struct {
	name Phase
	rows []stepRow
	// extra holds step ids the plan never listed — the engine and the plan
	// disagreeing degrades visibly instead of silently.
	extra      []string
	start, end time.Time
}

type streamEventMsg struct {
	ev Event
}

// StreamStep drives the deploy engine through the Execute hook and renders the
// phase checklist from its metrics-recorder event feed. It is forward-only: esc
// is ignored and ctrl+c becomes a graceful cancel (first press) then a force
// quit (second press).
type StreamStep struct {
	wizard.BaseStep
	st    *State
	hooks Hooks

	events           chan Event
	started          time.Time
	now              func() time.Time // overridden in tests for a deterministic elapsed reading
	startedGoroutine bool
	phases           []phaseProgress
	currentPhase     int
	cancelRequested  bool
	finished         bool
	loadingSpinner   spinner.Model
	// focusLine and lastLine are recorded during View: the running row's line,
	// and the last line of the rendered content.
	focusLine int
	lastLine  int

	boldStyle   lipgloss.Style
	doneStyle   lipgloss.Style
	failStyle   lipgloss.Style
	pendStyle   lipgloss.Style
	dimStyle    lipgloss.Style
	warnStyle   lipgloss.Style
	activeStyle lipgloss.Style
}

// NewStreamStep constructs the live deploy step.
func NewStreamStep(st *State, hooks Hooks) *StreamStep {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	return &StreamStep{
		BaseStep:       wizard.NewBaseStepWithDisplayTitle(StepIDStream, "install", "", ""),
		st:             st,
		hooks:          hooks,
		events:         make(chan Event, 64),
		now:            time.Now,
		loadingSpinner: sp,
		boldStyle:      lipgloss.NewStyle().Foreground(tui.ColorText).Bold(true),
		doneStyle:      lipgloss.NewStyle().Foreground(tui.ColorSuccess),
		failStyle:      lipgloss.NewStyle().Foreground(tui.ColorError),
		pendStyle:      lipgloss.NewStyle().Foreground(tui.ColorSlate600),
		dimStyle:       lipgloss.NewStyle().Foreground(tui.ColorSlate500),
		warnStyle:      lipgloss.NewStyle().Foreground(tui.ColorWarning),
		activeStyle:    lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true),
	}
}

// DisplayTitle names the header for the run in progress.
func (s *StreamStep) DisplayTitle() string {
	return s.progressLabel()
}

// Init groups the plan into phases, starts the engine goroutine exactly once,
// and begins listening for events.
func (s *StreamStep) Init() tea.Cmd {
	if s.startedGoroutine {
		return s.listen()
	}
	s.startedGoroutine = true
	s.st.Started = true
	s.started = s.now()
	s.buildRows()

	go func() {
		var err error
		if s.hooks.Execute != nil {
			err = s.hooks.Execute(s.st, s.events)
		}
		s.events <- Event{Final: true, Err: err}
	}()

	return tea.Batch(s.loadingSpinner.Tick, s.listen())
}

// buildRows groups the plan's steps into phases in execution order, dropping
// phases a resumed run skips entirely so they never sit pending forever.
func (s *StreamStep) buildRows() {
	byPhase := make(map[Phase][]stepRow, len(PhaseOrder()))
	for _, m := range s.st.Plan {
		phase, ok := PhaseOf(m.ID)
		if !ok {
			phase = Phase(m.Phase)
		}
		byPhase[phase] = append(byPhase[phase], stepRow{id: m.ID, label: m.Name})
	}

	s.phases = nil
	for _, phase := range PhaseOrder() {
		if rows := byPhase[phase]; len(rows) > 0 {
			s.phases = append(s.phases, phaseProgress{name: phase, rows: rows})
			delete(byPhase, phase)
		}
	}
	// An unclaimed phase (PhaseOf missed the step) still renders, after the
	// five known ones, rather than vanishing from the checklist.
	for phase, rows := range byPhase {
		s.phases = append(s.phases, phaseProgress{name: phase, rows: rows})
	}
}

func (s *StreamStep) listen() tea.Cmd {
	return func() tea.Msg { return streamEventMsg{ev: <-s.events} }
}

// Update consumes engine events and spinner ticks; every non-final event
// re-arms the listen command and nudges the viewport to follow the running row.
func (s *StreamStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case streamEventMsg:
		if msg.ev.Final {
			s.finished = true
			s.st.Executed = true
			s.st.Result = msg.ev.Err
			// The engine's own measurement wins when the hook reported one;
			// the screen's covers the runs that never got that far.
			if s.st.Elapsed == 0 {
				s.st.Elapsed = s.now().Sub(s.started)
			}
			s.finish(msg.ev.Err)
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDStream} }
		}
		s.applyEvent(&msg.ev)
		return s, tea.Batch(s.listen(), func() tea.Msg { return wizard.FocusChangedMsg{} })

	case spinner.TickMsg:
		if !s.finished {
			var cmd tea.Cmd
			s.loadingSpinner, cmd = s.loadingSpinner.Update(msg)
			return s, cmd
		}
	}
	return s, nil
}

// applyEvent updates phase/row state for ev: a phase change closes out the
// previous phase first, and a step the plan never listed degrades to the
// current phase's extra list.
func (s *StreamStep) applyEvent(ev *Event) {
	if len(s.phases) == 0 {
		return
	}
	pi, ri := s.locate(ev.StepID)
	if pi < 0 {
		if ev.StepID != "" && !ev.Done {
			s.phases[s.currentPhase].extra = append(s.phases[s.currentPhase].extra, string(ev.StepID))
		}
		return
	}
	if pi != s.currentPhase {
		s.closePhase(&s.phases[s.currentPhase])
	}
	s.currentPhase = pi
	ph := &s.phases[pi]
	if ph.start.IsZero() {
		ph.start = s.now()
	}

	row := &ph.rows[ri]
	switch {
	case ev.Done:
		row.status = doneStatus(ev)
		row.took = ev.Took
		s.markEarlierRowsDone(ph, ri)
	default:
		if row.status == rowPending {
			// A fresh row taking over "running" also takes over the extra
			// list: last render's leftover chatter belonged to the row that
			// just finished, not this one.
			ph.extra = nil
			row.status = rowRunning
			row.start = s.now()
		}
		s.markEarlierRowsDone(ph, ri)
	}
}

// doneStatus maps a closing event onto the row status it commits: a skipped
// step is neither a pass nor a failure, and an error is always a failure.
func doneStatus(ev *Event) rowStatus {
	switch {
	case ev.Err != nil:
		return rowFailed
	case ev.Skipped:
		return rowSkipped
	default:
		return rowDone
	}
}

// locate returns the phase and row indices for id, or (-1, -1) when no
// planned row owns it.
func (s *StreamStep) locate(id distribution.StepID) (phase, row int) {
	for i := range s.phases {
		for j := range s.phases[i].rows {
			if s.phases[i].rows[j].id == id {
				return i, j
			}
		}
	}
	return -1, -1
}

// closePhase stamps ph's end time and promotes any row still running or
// pending to done.
func (s *StreamStep) closePhase(ph *phaseProgress) {
	now := s.now()
	if ph.end.IsZero() {
		ph.end = now
	}
	for i := range ph.rows {
		promoteRowDone(&ph.rows[i], now)
	}
}

// markEarlierRowsDone marks rows before active done: the orchestrator runs
// steps strictly in order, so a later row starting implies the earlier ones
// finished.
func (s *StreamStep) markEarlierRowsDone(ph *phaseProgress, active int) {
	now := s.now()
	for i := range active {
		promoteRowDone(&ph.rows[i], now)
	}
}

// promoteRowDone closes a row that was still running or pending, backfilling a
// duration from its start when the caller never supplied one.
func promoteRowDone(r *stepRow, now time.Time) {
	if r.status != rowRunning && r.status != rowPending {
		return
	}
	if r.start.IsZero() {
		r.start = now
	}
	r.status = rowDone
	if r.took == 0 {
		r.took = now.Sub(r.start)
	}
}

// finish closes out the phase in flight when the run ends: a failure marks its
// running row failed, success closes it out like any earlier transition.
func (s *StreamStep) finish(err error) {
	if len(s.phases) == 0 {
		return
	}
	ph := &s.phases[s.currentPhase]
	if err != nil {
		now := s.now()
		for i := range ph.rows {
			r := &ph.rows[i]
			if r.status != rowRunning {
				continue
			}
			if r.start.IsZero() {
				r.start = now
			}
			r.status = rowFailed
			if r.took == 0 {
				r.took = now.Sub(r.start)
			}
		}
		return
	}
	s.closePhase(ph)
}

// ShouldShow keeps the step visible: the flow is only ever entered to run a
// deploy the operator already consented to.
func (s *StreamStep) ShouldShow(_ *config.Config) bool {
	return true
}

// InterceptBack makes the deploy screen forward-only: esc must never orphan
// the event pump mid-install (the engine goroutine would block on a full
// channel) or re-arm a listener on a finished run.
func (s *StreamStep) InterceptBack() bool {
	return true
}

// InterceptQuit turns the first ctrl+c into a graceful cancel (the resume
// marker stays, the engine unwinds) and lets the second one force-quit.
func (s *StreamStep) InterceptQuit() bool {
	if s.cancelRequested || s.finished {
		return false
	}
	s.cancelRequested = true
	if s.hooks.CancelDeploy != nil {
		s.hooks.CancelDeploy()
	}
	return true
}

// View renders the phase checklist: a finished phase collapses to a single line
// with its total, the running phase stays expanded with right-aligned
// durations and a live elapsed reading, and untouched phases show a bare
// pending bullet.
func (s *StreamStep) View(width, height int) string {
	s.SetSize(width, height)
	col := max(width-4, 1)

	lines := []string{s.headline(col)}
	if s.cancelRequested && !s.finished {
		lines = append(lines, s.warnStyle.Render(lipgloss.Wrap(
			"cancel requested — finishing the current step safely, the resume marker stays…", col, "")))
	}

	s.focusLine = -1
	for i := range s.phases {
		lines = s.appendPhase(lines, i, col)
	}

	footnote := "full log " + logutil.DefaultLogFileName + " · ctrl+c cancels after the current step"
	lines = append(lines, "", s.dimStyle.Render(lipgloss.Wrap(footnote, col, "")))

	content := strings.Join(lines, "\n")
	s.lastLine = strings.Count(content, "\n")
	return content
}

// headline renders the run's step progress and its live elapsed time,
// right-aligned at col.
func (s *StreamStep) headline(col int) string {
	done, total := s.stepCounts()
	left := fmt.Sprintf("%s  %d / %d", s.progressLabel(), done, max(total, 1))
	right := "elapsed " + fmtDur(s.now().Sub(s.started))
	return justify(s.boldStyle.Render(left), s.dimStyle.Render(right), col)
}

// progressLabel names the run in the header and the headline.
func (s *StreamStep) progressLabel() string {
	if s.st.Cfg != nil && s.st.Cfg.Cluster.Name != "" {
		return "deploying " + s.st.Cfg.Cluster.Name
	}
	return "deploying cluster"
}

// stepCounts reports how many planned steps have settled and how many there
// are; a running row counts as unsettled.
func (s *StreamStep) stepCounts() (done, total int) {
	for i := range s.phases {
		for j := range s.phases[i].rows {
			total++
			if s.phases[i].rows[j].status != rowPending && s.phases[i].rows[j].status != rowRunning {
				done++
			}
		}
	}
	return done, total
}

// appendPhase renders phase i onto lines: collapsed with its total once passed
// (or once the run has finished cleanly), expanded with its rows while
// current, or a bare pending bullet otherwise.
func (s *StreamStep) appendPhase(lines []string, i, col int) []string {
	ph := &s.phases[i]
	switch {
	case i < s.currentPhase || (s.finished && s.st.Result == nil):
		return append(lines, justify(
			s.doneStyle.Render(tui.IconSuccess+" "+string(ph.name)),
			s.dimStyle.Render(fmtDur(ph.end.Sub(ph.start))),
			col,
		))
	case i == s.currentPhase || phaseTouched(ph):
		lines = append(lines, s.activeStyle.Render(tui.IconActive+" "+string(ph.name)))
		for j := range ph.rows {
			r := &ph.rows[j]
			if r.status == rowRunning {
				s.focusLine = len(lines)
			}
			lines = append(lines, "    "+s.renderRow(r, col-4))
			if r.status == rowRunning && len(ph.extra) > 0 {
				lines = append(lines, "    "+s.dimStyle.MaxWidth(col-4).Render("… "+ph.extra[len(ph.extra)-1]))
			}
		}
		return lines
	default:
		return append(lines, s.pendStyle.Render(tui.IconPending+" "+string(ph.name)))
	}
}

// renderRow renders one step row: settled rows show their duration (or
// "skipped") right-aligned, the running row shows a live one, pending rows
// show neither.
func (s *StreamStep) renderRow(r *stepRow, col int) string {
	switch r.status {
	case rowDone:
		return justify(s.doneStyle.Render(tui.IconSuccess+" "+r.label), s.dimStyle.Render(fmtDur(r.took)), col)
	case rowSkipped:
		return justify(s.dimStyle.Render(tui.IconSkip+" "+r.label), s.dimStyle.Render("skipped"), col)
	case rowFailed:
		return justify(s.failStyle.Render(tui.IconError+" "+r.label), s.dimStyle.Render(fmtDur(r.took)), col)
	case rowRunning:
		return justify(s.loadingSpinner.View()+s.boldStyle.Render(r.label), s.dimStyle.Render(fmtDur(s.now().Sub(r.start))), col)
	default:
		return s.pendStyle.Render(tui.IconPending + " " + r.label)
	}
}

func phaseTouched(ph *phaseProgress) bool {
	for i := range ph.rows {
		if ph.rows[i].status != rowPending {
			return true
		}
	}
	return len(ph.extra) > 0
}

// FocusedSpan reports the running row's line, falling back to the last
// rendered line once the run has finished or no row is running.
func (s *StreamStep) FocusedSpan() (wizard.LineSpan, bool) {
	if len(s.phases) == 0 {
		return wizard.LineSpan{}, false
	}
	line := s.focusLine
	if s.finished || line < 0 {
		line = s.lastLine
	}
	return wizard.LineSpan{Start: line, End: line}, true
}

// justify right-aligns right within width, ANSI-safely truncating left (never
// padding it) so the combined line is exactly width columns wide; an oversized
// right on its own is clamped rather than left to overflow.
func justify(left, right string, width int) string {
	rightW := lipgloss.Width(right)
	if rightW > width {
		return tui.Truncate(right, width)
	}
	leftW := max(width-rightW-1, 0)
	left = lipgloss.NewStyle().MaxWidth(leftW).Render(left)
	gap := max(width-lipgloss.Width(left)-rightW, 0)
	return left + strings.Repeat(" ", gap) + right
}

// fmtDur renders d truncated to whole seconds, the deploy screen's duration format.
func fmtDur(d time.Duration) string {
	return d.Truncate(time.Second).String()
}

// ShortHelp explains the constrained keys: no esc, guarded ctrl+c.
func (s *StreamStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: wizard.HelpCtrlC, Help: "request graceful cancel (twice to force-quit)"},
	}
}
