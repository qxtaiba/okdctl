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
	// implied marks a row whose completion was inferred (never started, then
	// promoted) rather than measured; rowDur renders it "—" instead of a
	// fabricated 0s.
	implied bool
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
	frameSize
	st    *State
	hooks Hooks
	log   logView

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
	// and the last line of the rendered content. tailRendered records whether
	// that render put the log tail under the checklist.
	focusLine    int
	lastLine     int
	tailRendered bool
	// viewCol, fullLogHeight, paneWidth, and paneHeight are the log window's
	// geometry as last rendered — View and PaneContent record them so a paging
	// key moves by exactly the window the operator is looking at.
	viewCol       int
	fullLogHeight int
	paneWidth     int
	paneHeight    int

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
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary())

	return &StreamStep{
		BaseStep:       wizard.NewBaseStepWithDisplayTitle(StepIDStream, "install", "", ""),
		st:             st,
		hooks:          hooks,
		events:         make(chan Event, 64),
		now:            time.Now,
		loadingSpinner: sp,
		boldStyle:      lipgloss.NewStyle().Foreground(tui.ColorText()).Bold(true),
		doneStyle:      lipgloss.NewStyle().Foreground(tui.ColorSuccess()),
		failStyle:      lipgloss.NewStyle().Foreground(tui.ColorError()),
		pendStyle:      lipgloss.NewStyle().Foreground(tui.ColorSubtle()),
		dimStyle:       lipgloss.NewStyle().Foreground(tui.ColorTextFaint()),
		warnStyle:      lipgloss.NewStyle().Foreground(tui.ColorWarning()),
		activeStyle:    lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true),
	}
}

// SetSize records the body box the frame gives the step: the full-screen log
// sizes itself to that height, which View's own fixed 1000-row budget cannot
// report.
func (s *StreamStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	s.bodyHeight = height
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
		s.sendFinal(err)
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

// sendFinal delivers the run's terminal event, abandoning it once the run's
// context is gone: the engine goroutine must not outlive a force-quit waiting on
// a feed nobody drains.
func (s *StreamStep) sendFinal(err error) {
	select {
	case <-s.hooks.Done:
	case s.events <- Event{Final: true, Err: err}:
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

	case tea.KeyPressMsg:
		cmd := s.handleLogKey(msg)
		return s, cmd
	}
	return s, nil
}

// handleLogKey binds the log viewport's keys: `l` locks the window where it
// stands (or releases it back to the tail), `f` swaps the log full-screen and
// back (which changes the frame's own layout gate and so asks for a
// re-measure), pgup/pgdn page the window through the whole ring wherever
// ConsumesPaging routes them here, and the arrows walk it line by line in
// full-screen mode. With no Logs hook (documented-supported) every one of
// them is inert — an empty full-screen log would blank the whole body.
func (s *StreamStep) handleLogKey(msg tea.KeyPressMsg) tea.Cmd {
	if s.hooks.Logs == nil {
		return nil
	}
	switch msg.Code {
	case keyLogLock:
		s.log.locked = !s.log.locked
		if s.log.locked {
			s.log.lockAt = lockedAt(s.hooks.Logs)
		}
	case keyLogFull:
		s.log.full = !s.log.full
		return func() tea.Msg { return wizard.LayoutChangedMsg{} }
	case tea.KeyPgUp:
		s.scrollLogBy(-s.logPageSize())
	case tea.KeyPgDown:
		s.scrollLogBy(s.logPageSize())
	case tea.KeyUp:
		if s.log.full {
			s.scrollLogBy(-1)
		}
	case tea.KeyDown:
		if s.log.full {
			s.scrollLogBy(1)
		}
	}
	return nil
}

// ConsumesPaging reports whether pgup/pgdn page the log window itself — the
// full-screen log always, a locked pane or tail too — so the frame leaves the
// keys to the step instead of scrolling the checklist viewport.
func (s *StreamStep) ConsumesPaging() bool {
	return s.hooks.Logs != nil && (s.log.full || s.log.locked)
}

// ScrollsWithArrows opts the checklist into the frame's line-by-line arrow
// scroll; in full-screen mode the arrows fall through to handleLogKey and walk
// the log instead.
func (s *StreamStep) ScrollsWithArrows() bool {
	return s.hooks.Logs == nil || !s.log.full
}

// scrollLogBy moves the log window n lines through the ring at the geometry
// last rendered, flooring at the stream's oldest full window.
func (s *StreamStep) scrollLogBy(n int) {
	w, h, wrap := s.logGeometry()
	scrollLog(&s.log, s.hooks.Logs, n, topLogLines(s.hooks.Logs, w, h, wrap))
}

// logPageSize is how many lines one pgup/pgdn moves: exactly the lines the
// active window is showing, so a page never skips past unread ones.
func (s *StreamStep) logPageSize() int {
	w, h, wrap := s.logGeometry()
	return visibleLogLines(s.hooks.Logs, s.log, w, h, wrap)
}

// logGeometry names the active log window: the full-screen box, the split
// pane, or the narrow tail under the checklist.
func (s *StreamStep) logGeometry() (width, height int, wrap bool) {
	switch {
	case s.log.full:
		return max(s.viewCol, 1), max(s.fullLogHeight, 2), true
	case s.paneCarriesLog():
		return max(s.paneWidth, 1), max(s.paneHeight, 2), false
	default:
		return max(s.viewCol, 1), narrowTailRows + 1, false
	}
}

// SuppressesSplit hands the log the whole frame while `f` has it full-screen;
// the checklist comes back the moment it is toggled off.
func (s *StreamStep) SuppressesSplit() bool {
	return s.log.full && s.hooks.Logs != nil
}

// PaneContent fills the split layout's right pane with the live log, in place of
// the context pane's step list.
func (s *StreamStep) PaneContent(width, height int) string {
	s.paneWidth, s.paneHeight = width, height
	return renderLogPane(s.hooks.Logs, s.log, width, height, false)
}

// paneCarriesLog reports whether the log has a pane of its own, in which case
// the checklist body carries no tail.
func (s *StreamStep) paneCarriesLog() bool {
	return s.hooks.Logs != nil && !s.log.full && s.splitsFrame()
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
// duration from its start when the caller never supplied one; a row that
// never started is marked implied so its duration renders "—".
func promoteRowDone(r *stepRow, now time.Time) {
	if r.status != rowRunning && r.status != rowPending {
		return
	}
	if r.start.IsZero() {
		r.start = now
		r.implied = true
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
// pending bullet. The height argument is the frame's fixed 1000-row scratch
// budget, never the body's real height — SetSize records that.
func (s *StreamStep) View(width, _ int) string {
	col := max(width-4, 1)
	s.viewCol = col
	if s.log.full {
		// The headline rides above the full-screen log: progress and elapsed are
		// what an operator would otherwise lose by leaving the checklist. The
		// sink path rides with it — the file keeps every byte the ring evicts.
		head := []string{s.headline(col)}
		if s.hooks.LogPath != "" {
			head = append(head, s.dimStyle.Render(tui.Truncate("full log: "+s.hooks.LogPath, col)))
		}
		s.fullLogHeight = max(s.bodyHeight-len(head), 2)
		return strings.Join(head, "\n") + "\n" + renderLogFull(s.hooks.Logs, s.log, col, s.fullLogHeight)
	}

	lines := []string{s.headline(col)}
	if s.cancelRequested && !s.finished {
		lines = append(lines, s.warnStyle.Render(lipgloss.Wrap(
			"cancel requested — finishing the current step safely, the resume marker stays…", col, "")))
	}

	s.focusLine = -1
	for i := range s.phases {
		lines = s.appendPhase(lines, i, col)
	}

	s.tailRendered = false
	if !s.paneCarriesLog() {
		if tail := renderLogTail(s.hooks.Logs, s.log, col, narrowTailRows); len(tail) > 0 {
			lines = append(lines, "")
			lines = append(lines, tail...)
			s.tailRendered = true
		}
	}

	// The cancel clause holds only while a first ctrl+c would still cancel
	// gracefully; after a cancel (or completion) the next ctrl+c force-quits,
	// and the promise would be a lie. The sink clause names the real resolved
	// path, or nothing at all when no file sink is open — never a guess.
	var clauses []string
	if s.hooks.LogPath != "" {
		clauses = append(clauses, "full log "+s.hooks.LogPath)
	}
	if !s.finished && !s.cancelRequested {
		clauses = append(clauses, "ctrl+c cancels after the current step")
	}
	if len(clauses) > 0 {
		lines = append(lines, "", s.dimStyle.Render(lipgloss.Wrap(strings.Join(clauses, " · "), col, "")))
	}

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
		return justify(s.doneStyle.Render(tui.IconSuccess+" "+r.label), s.dimStyle.Render(rowDur(r)), col)
	case rowSkipped:
		return justify(s.dimStyle.Render(tui.IconSkip+" "+r.label), s.dimStyle.Render("skipped"), col)
	case rowFailed:
		return justify(s.failStyle.Render(tui.IconError+" "+r.label), s.dimStyle.Render(rowDur(r)), col)
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

// FocusedSpan reports the running row's line, falling back to the last rendered
// line once the run has finished or no row is running. A narrow frame follows
// the tail instead: the log rides at the bottom of the body, and a checklist
// longer than the viewport would otherwise park the newest line below the fold,
// leaving the operator scrolling for the one thing still moving.
func (s *StreamStep) FocusedSpan() (wizard.LineSpan, bool) {
	if len(s.phases) == 0 {
		return wizard.LineSpan{}, false
	}
	if s.tailRendered {
		return wizard.LineSpan{Start: s.lastLine, End: s.lastLine}, true
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

// rowDur renders a settled row's duration: "—" for one whose completion was
// inferred rather than measured, "<1s" for a real sub-second measurement.
func rowDur(r *stepRow) string {
	if r.implied {
		return "—"
	}
	if r.took < time.Second {
		return "<1s"
	}
	return fmtDur(r.took)
}

// fmtDur renders d truncated to whole seconds, the deploy screen's duration format.
func fmtDur(d time.Duration) string {
	return d.Truncate(time.Second).String()
}

// ShortHelp explains the log viewport's keys plus the constrained ones: no esc,
// guarded ctrl+c. The ribbon reserves only ctrl+c and "?" and then drops the
// rest front to back, so in full-screen mode f leads the list — it is the only
// way back to the checklist, and losing it would strand the operator on the log.
func (s *StreamStep) ShortHelp() []wizard.KeyBinding {
	cancel := wizard.KeyBinding{Key: wizard.HelpCtrlC, Help: "cancel (twice to force-quit)"}
	if s.hooks.Logs == nil {
		return []wizard.KeyBinding{cancel}
	}
	lock := wizard.KeyBinding{Key: string(keyLogLock), Help: s.lockHelp()}
	full := wizard.KeyBinding{Key: string(keyLogFull), Help: s.fullHelp()}
	page := wizard.KeyBinding{Key: "pgup/pgdn", Help: "page the log"}
	if s.log.full {
		return []wizard.KeyBinding{full, lock, page, cancel}
	}
	if s.log.locked {
		return []wizard.KeyBinding{lock, full, page, cancel}
	}
	return []wizard.KeyBinding{lock, full, cancel}
}

// lockHelp and fullHelp name what the key does next, not what it did, so the
// ribbon reads as an instruction in either state.
func (s *StreamStep) lockHelp() string {
	if s.log.locked {
		return "follow the log tail"
	}
	return "lock the log here"
}

func (s *StreamStep) fullHelp() string {
	if s.log.full {
		return "back to checklist"
	}
	return "full-screen log"
}
