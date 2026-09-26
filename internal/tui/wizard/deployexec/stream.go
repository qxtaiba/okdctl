package deployexec

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
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
	wizard.FrameSize
	st    *State
	hooks Hooks
	log   logview.Surface

	events           chan Event
	started          time.Time
	now              func() time.Time // overridden in tests for a deterministic elapsed reading
	startedGoroutine bool
	phases           []phaseProgress
	currentPhase     int
	cancelRequested  bool
	finished         bool
	frame            uint64

	// The install instrument's state: the weight table seeded from
	// persisted history, the monotonic progress fraction, the frame-driven
	// settle/sweep easing, the ETA correction, and the activity meter.
	weights     map[distribution.StepID]float64
	totalWeight float64
	hasHistory  bool
	maxFrac     float64
	settling    bool
	sweeping    bool
	blurred     bool
	animFrom    float64
	animStart   uint64
	etaShown    time.Duration
	ewmaRatio   float64
	// lastSeenFrac is where the bar stood when the terminal blurred — the
	// catch-up sweep's origin; stallRung debounces the away-mode stall bell.
	lastSeenFrac float64
	stallRung    bool
	// activityCount advances one unit per observed event or log line;
	// lastActivity stamps the newest one, lastLogTotal the ring position
	// already folded in.
	activityCount uint64
	lastActivity  time.Time
	lastLogTotal  int64
	// focusLine and lastLine are recorded during View: the running row's line,
	// and the last line of the rendered content. tailRendered records whether
	// that render put the log tail under the checklist.
	focusLine    int
	lastLine     int
	tailRendered bool

	wizard.ExecStyleCache
}

// NewStreamStep constructs the live deploy step.
func NewStreamStep(st *State, hooks Hooks) *StreamStep {
	return &StreamStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(StepIDStream, "install", "", ""),
		st:       st,
		hooks:    hooks,
		log:      logview.Surface{Src: hooks.Logs},
		events:   make(chan Event, 64),
		now:      time.Now,
	}
}

// SetSize records the body box the frame gives the step: the full-screen log
// sizes itself to that height, which View's own fixed 1000-row budget cannot
// report.
func (s *StreamStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	s.SetBodyHeight(height)
}

// DisplayTitle names the header for the run in progress.
func (s *StreamStep) DisplayTitle() string {
	return s.progressLabel()
}

// Animating reports whether the screen needs frame ticks: any live run,
// plus the settle window after a clean finish.
func (s *StreamStep) Animating() bool {
	return !s.finished || s.settling
}

// WindowTitle carries the run's live progress into the terminal tab — the
// weight model's percent plus the running phase — so a backgrounded install
// stays legible from the tab bar; empty once the run ends, falling back to
// the header title.
func (s *StreamStep) WindowTitle() string {
	if s.finished || len(s.phases) == 0 {
		return ""
	}
	return fmt.Sprintf("deploying %d%% · %s", s.percent(), s.phases[s.currentPhase].name)
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

	return s.listen()
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

	s.buildWeights()
	s.ewmaRatio = 1
	s.lastActivity = s.started
}

// sendFinal delivers the run's terminal event, abandoning it only once the
// run's context is gone AND the feed cannot accept it: the engine goroutine
// must not outlive a force-quit waiting on a feed nobody drains. Delivery is
// biased — a graceful cancel closes the same channel this select watches,
// and a uniform two-way select would drop the final event about half the
// time, stranding the screen on "cancel requested" forever.
func (s *StreamStep) sendFinal(err error) {
	ev := Event{Final: true, Err: err}
	select {
	case s.events <- ev:
	default:
		select {
		case <-s.hooks.Done:
		case s.events <- ev:
		}
	}
}

func (s *StreamStep) listen() tea.Cmd {
	return func() tea.Msg { return streamEventMsg{ev: <-s.events} }
}

// Update consumes engine events and shared-clock frames; every non-final
// event re-arms the listen command and nudges the viewport to follow the
// running row.
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
			// A clean finish under full motion holds the screen for the
			// settle to 100% — the completion fires from the settle's last
			// frame. A failure, a blurred terminal (1Hz clock), and the
			// calmer dials complete immediately.
			if msg.ev.Err == nil && tui.Motion() == tui.MotionFull && !s.blurred {
				s.settling = true
				s.animFrom = min(s.maxFrac, barCapFrac)
				s.animStart = s.frame
				return s, nil
			}
			complete := func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDStream} }
			if bell := s.bellWhileBlurred(); bell != nil {
				return s, tea.Batch(bell, complete)
			}
			return s, complete
		}
		s.sampleActivity()
		s.applyEvent(&msg.ev)
		return s, tea.Batch(s.listen(), func() tea.Msg { return wizard.FocusChangedMsg{} })

	case wizard.FrameMsg:
		s.frame = msg.Frame
		s.sampleActivity()
		if s.settling && s.frame >= s.animStart+settleFrames {
			s.settling = false
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDStream} }
		}
		if s.sweeping && s.frame >= s.animStart+settleFrames {
			s.sweeping = false
		}
		bell := s.stallBell()
		return s, bell

	case tea.BlurMsg:
		// Away mode: remember where the bar stood so a later focus can
		// sweep the difference; the cosmetic animators check the flag.
		s.blurred = true
		s.lastSeenFrac = s.barFillFrac()

	case tea.FocusMsg:
		wasBlurred := s.blurred
		s.blurred = false
		if wasBlurred && tui.Motion() == tui.MotionFull && !s.finished && s.lastSeenFrac < s.barTarget() {
			// The catch-up sweep: one eased pass from the last-seen fill to
			// the current percent — an instant visual diff of the time away.
			s.sweeping = true
			s.animFrom = s.lastSeenFrac
			s.animStart = s.frame
		}

	case tea.KeyPressMsg:
		cmd := s.handleLogKey(msg)
		return s, cmd
	}
	return s, nil
}

// handleLogKey routes the log viewport's keys through the shared surface;
// only `f` needs a command back — swapping the log full-screen changes the
// frame's own layout gate and so asks for a re-measure. With no Logs hook
// (documented-supported) every key is inert — an empty full-screen log
// would blank the whole body.
func (s *StreamStep) handleLogKey(msg tea.KeyPressMsg) tea.Cmd {
	if s.log.HandleKey(msg, s.paneCarriesLog()) {
		return func() tea.Msg { return wizard.LayoutChangedMsg{} }
	}
	return nil
}

// ConsumesPaging reports whether pgup/pgdn page the log window itself — the
// full-screen log always, a locked pane or tail too — so the frame leaves the
// keys to the step instead of scrolling the checklist viewport.
func (s *StreamStep) ConsumesPaging() bool {
	return s.log.ConsumesPaging()
}

// ScrollsWithArrows opts the checklist into the frame's line-by-line arrow
// scroll; in full-screen mode the arrows fall through to handleLogKey and walk
// the log instead.
func (s *StreamStep) ScrollsWithArrows() bool {
	return s.hooks.Logs == nil || !s.log.Full()
}

// SuppressesSplit hands the log the whole frame while `f` has it full-screen;
// the checklist comes back the moment it is toggled off.
func (s *StreamStep) SuppressesSplit() bool {
	return s.log.Full() && s.hooks.Logs != nil
}

// PaneContent fills the split layout's right pane with the live log, in place of
// the context pane's step list.
func (s *StreamStep) PaneContent(width, height int) string {
	return s.log.RenderPane(width, height)
}

// paneCarriesLog reports whether the log has a pane of its own, in which case
// the checklist body carries no tail.
func (s *StreamStep) paneCarriesLog() bool {
	return s.hooks.Logs != nil && !s.log.Full() && s.SplitsFrame(flowStepCount)
}

// applyEvent updates phase/row state for ev: a phase change closes out the
// previous phase first, and a step the plan never listed degrades to the
// current phase's extra list.
func (s *StreamStep) applyEvent(ev *Event) {
	if len(s.phases) == 0 {
		return
	}
	s.noteActivity()
	if ev.Done && !ev.Skipped && ev.Took > 0 && s.hasHistory {
		// One settled step's actual-vs-schedule ratio corrects the ETA.
		if predicted, ok := s.weights[ev.StepID]; ok && predicted > 0 {
			s.ewmaRatio = etaEWMAAlpha*(ev.Took.Seconds()/predicted) + (1-etaEWMAAlpha)*s.ewmaRatio
		}
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
	s.log.ViewCol = col
	s.updateETAShown()
	if s.log.Full() {
		// The headline and the bar ride above the full-screen log: progress
		// and elapsed are what an operator would otherwise lose by leaving
		// the checklist. The sink path rides with them — the file keeps
		// every byte the ring evicts.
		head := []string{s.headline(col)}
		if bar := s.renderBar(col); bar != "" {
			head = append(head, bar)
		}
		if s.hooks.LogPath != "" {
			head = append(head, s.Styles().Dim.Render(tui.Truncate("full log: "+s.hooks.LogPath, col)))
		}
		return strings.Join(head, "\n") + "\n" + s.log.RenderFull(col, max(s.BodyHeight()-len(head), 2))
	}

	lines := []string{s.headline(col)}
	if bar := s.renderBar(col); bar != "" {
		lines = append(lines, bar)
	}
	if s.cancelRequested && !s.finished {
		lines = append(lines, s.Styles().Warn.Render(lipgloss.Wrap(
			"cancel requested — finishing the current step safely, the resume marker stays…", col, "")))
	}

	s.focusLine = -1
	for i := range s.phases {
		lines = s.appendPhase(lines, i, col)
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

	// On the split tier the pane carries the log, so the checklist's dead
	// lower rows take the per-phase duration bars instead; a narrow frame
	// spends the same slack on the log tail below.
	if s.paneCarriesLog() {
		chrome := 2
		if len(clauses) > 0 {
			chrome += 2
		}
		if s.BodyHeight()-len(lines)-chrome >= s.phaseBarRows() {
			lines = append(lines, s.renderPhaseBars(col)...)
		}
	}

	s.tailRendered = false
	if !s.paneCarriesLog() {
		// The tail's budget is whatever body rows the checklist and the
		// chrome around the tail (its blank row, the LOG header, and the
		// clauses block) leave over, floored at logview.NarrowTailRows —
		// slack becomes evidence instead of blank rows.
		chrome := 2
		if len(clauses) > 0 {
			chrome += 2
		}
		budget := max(logview.NarrowTailRows, s.BodyHeight()-len(lines)-chrome)
		if tail := s.log.RenderTail(col, budget); len(tail) > 0 {
			lines = append(lines, "")
			lines = append(lines, tail...)
			s.tailRendered = true
		}
	}

	if len(clauses) > 0 {
		lines = append(lines, "", s.Styles().Dim.Render(lipgloss.Wrap(strings.Join(clauses, " · "), col, "")))
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
	return justify(s.Styles().Bold.Render(left), s.Styles().Dim.Render(right), col)
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
			s.Styles().Done.Render(tui.IconSuccess+" "+string(ph.name)),
			s.Styles().Dim.Render(fmtDur(ph.end.Sub(ph.start))),
			col,
		))
	case i == s.currentPhase || phaseTouched(ph):
		lines = append(lines, s.Styles().Active.Render(tui.IconActive+" "+string(ph.name)))
		for j := range ph.rows {
			r := &ph.rows[j]
			if r.status == rowRunning {
				s.focusLine = len(lines)
			}
			lines = append(lines, "    "+s.renderRow(r, col-4))
			if r.status == rowRunning && len(ph.extra) > 0 {
				lines = append(lines, "    "+s.Styles().Dim.MaxWidth(col-4).Render("… "+ph.extra[len(ph.extra)-1]))
			}
		}
		return lines
	default:
		label := s.Styles().Pend.Render(tui.IconPending + " " + string(ph.name))
		// The schedule annotation: unstarted phases carry their historical
		// durations, so the trail reads as a plan, not a mystery.
		if d := s.phasePredicted(ph); d > 0 {
			return append(lines, justify(label, s.Styles().Dim.Render("~"+fmtETA(d)), col))
		}
		return append(lines, label)
	}
}

// renderRow renders one step row: settled rows show their duration (or
// "skipped") right-aligned, the running row shows a live one, pending rows
// show neither.
func (s *StreamStep) renderRow(r *stepRow, col int) string {
	switch r.status {
	case rowDone:
		return justify(s.Styles().Done.Render(tui.IconSuccess+" "+r.label), s.Styles().Dim.Render(rowDur(r)), col)
	case rowSkipped:
		return justify(s.Styles().Dim.Render(tui.IconSkip+" "+r.label), s.Styles().Dim.Render("skipped"), col)
	case rowFailed:
		return justify(s.Styles().Fail.Render(tui.IconError+" "+r.label), s.Styles().Dim.Render(rowDur(r)), col)
	case rowRunning:
		if s.stalled(r.id) {
			return justify(s.stallMarker()+s.Styles().Warn.Render(r.label),
				s.Styles().Warn.Render("last output "+fmtAgo(s.now().Sub(s.lastActivity))+" ago"), col)
		}
		return justify(s.runningGlyph()+s.Styles().Bold.Render(r.label), s.Styles().Dim.Render(fmtDur(s.now().Sub(r.start))), col)
	default:
		return s.Styles().Pend.Render(tui.IconPending + " " + r.label)
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
	lock := wizard.KeyBinding{Key: string(rune(logview.KeyLock)), Help: s.log.LockHelp()}
	full := wizard.KeyBinding{Key: string(rune(logview.KeyFull)), Help: s.log.FullHelp()}
	page := wizard.KeyBinding{Key: "pgup/pgdn", Help: "page the log"}
	if s.log.Full() {
		return []wizard.KeyBinding{full, lock, page, cancel}
	}
	if s.log.Locked() {
		return []wizard.KeyBinding{lock, full, page, cancel}
	}
	return []wizard.KeyBinding{lock, full, cancel}
}
