package lifecycle

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

type rowStatus int

const (
	rowPending rowStatus = iota
	rowRunning
	rowDone
	rowFailed
)

type execRow struct {
	label  string
	status rowStatus
	start  time.Time
	took   time.Duration
	// implied marks a row whose completion was inferred (never started, then
	// promoted) rather than measured; rowDur renders it "—" instead of a
	// fabricated 0s.
	implied bool
}

type nodeProgress struct {
	name string
	rows []execRow
	// extra: unmatched Reporter descriptions — degrades visibly instead of silently.
	extra      []string
	start, end time.Time
}

type execEventMsg struct {
	ev ExecEvent
}

// ExecStep drives the approved operation through the Execute hook and
// renders per-node gate progress from the OnStep/Reporter event feed. It
// is forward-only: esc is ignored and ctrl+c becomes a graceful cancel
// (first press) then a force quit (second press).
type ExecStep struct {
	wizard.BaseStep
	wizard.FrameSize
	st    *State
	hooks Hooks
	log   logview.Surface

	events           chan ExecEvent
	started          time.Time
	now              func() time.Time // overridden in tests for a deterministic elapsed reading
	startedGoroutine bool
	nodes            []nodeProgress
	currentNode      int
	cancelRequested  bool
	finished         bool
	frame            uint64
	// focusLine and lastLine are recorded during View: the running row's
	// line, and the last line of the rendered content. tailRendered records
	// whether that render put the log tail under the checklist.
	focusLine    int
	lastLine     int
	tailRendered bool

	wizard.ExecStyleCache
}

// NewExecStep constructs the live execution step.
func NewExecStep(st *State, hooks Hooks) *ExecStep {
	return &ExecStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(StepIDExec,
			"execute", "", ""),
		st:     st,
		hooks:  hooks,
		log:    logview.Surface{Src: hooks.Logs},
		events: make(chan ExecEvent, 32),
		now:    time.Now,
	}
}

// SetSize records the body box the frame gives the step: the full-screen
// log and the tail budget size themselves to that height, which View's own
// fixed 1000-row budget cannot report.
func (s *ExecStep) SetSize(width, height int) {
	s.BaseStep.SetSize(width, height)
	s.SetBodyHeight(height)
}

// DisplayTitle names the header for the operation in progress.
func (s *ExecStep) DisplayTitle() string {
	return opProgressLabel(s.st.Op, s.execRole())
}

// ShouldShow gates the step to consented plans.
func (s *ExecStep) ShouldShow(_ *config.Config) bool {
	return s.st.Proceed
}

// Animating reports whether the running row's spinner needs frame ticks.
func (s *ExecStep) Animating() bool {
	return !s.finished
}

// Init derives the checklist from the plan, starts the Runner goroutine
// exactly once, and begins listening for events.
func (s *ExecStep) Init() tea.Cmd {
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

func (s *ExecStep) buildRows() {
	rows := GateRows(s.st.Op, s.execRole(), s.st.SkipDrain, diskModeFor(s.st))
	if s.st.Plan == nil {
		return
	}
	s.nodes = make([]nodeProgress, len(s.st.Plan.Nodes))
	for i := range s.st.Plan.Nodes {
		np := nodeProgress{name: s.st.Plan.Nodes[i].Name}
		np.rows = make([]execRow, len(rows))
		for j, r := range rows {
			np.rows[j] = execRow{label: r}
		}
		s.nodes[i] = np
	}
}

func (s *ExecStep) execRole() nodetypes.NodeRole {
	if s.st.Plan != nil && len(s.st.Plan.Nodes) > 0 && s.st.Plan.Nodes[0].Role != "" {
		return s.st.Plan.Nodes[0].Role
	}
	if s.st.Scope.Role != "" {
		return s.st.Scope.Role
	}
	return nodetypes.RoleWorker
}

// sendFinal delivers the run's terminal event, abandoning it only once the
// op's context is gone AND the feed cannot accept it: the runner goroutine
// holds the run lock and must not outlive a force-quit waiting on a feed
// nobody drains. Delivery is biased — the graceful cancel cancels the same
// context this select watches, and a uniform two-way select would drop the
// final event about half the time, stranding the exec screen on "cancel
// requested — finishing safely…" until a forced second ctrl+c.
func (s *ExecStep) sendFinal(err error) {
	ev := ExecEvent{Final: true, Err: err}
	select {
	case s.events <- ev:
	default:
		select {
		case <-s.hooks.Done:
		case s.events <- ev:
		}
	}
}

func (s *ExecStep) listen() tea.Cmd {
	return func() tea.Msg { return execEventMsg{ev: <-s.events} }
}

// Update consumes execution events and shared-clock frames; every
// non-final event re-arms the listen command and nudges the viewport to
// follow the running row.
func (s *ExecStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case execEventMsg:
		if msg.ev.Final {
			s.finished = true
			s.st.Executed = true
			s.st.Result = msg.ev.Err
			s.st.Elapsed = s.now().Sub(s.started)
			s.finish(msg.ev.Err)
			s.st.frozen, s.st.frozenAt = s.nodes, s.currentNode
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDExec} }
		}
		s.applyEvent(&msg.ev)
		return s, tea.Batch(s.listen(), func() tea.Msg { return wizard.FocusChangedMsg{} })

	case wizard.FrameMsg:
		s.frame = msg.Frame

	case tea.KeyPressMsg:
		cmd := s.handleLogKey(msg)
		return s, cmd
	}
	return s, nil
}

// handleLogKey routes the log viewport's keys through the shared surface;
// only `f` needs a command back — swapping the log full-screen changes the
// frame's own layout gate and so asks for a re-measure. With no Logs hook
// every key is inert — an empty full-screen log would blank the whole body.
func (s *ExecStep) handleLogKey(msg tea.KeyPressMsg) tea.Cmd {
	if s.log.HandleKey(msg, s.paneCarriesLog()) {
		return func() tea.Msg { return wizard.LayoutChangedMsg{} }
	}
	return nil
}

// ConsumesPaging reports whether pgup/pgdn page the log window itself — the
// full-screen log always, a locked pane or tail too — so the frame leaves
// the keys to the step instead of scrolling the checklist viewport.
func (s *ExecStep) ConsumesPaging() bool {
	return s.log.ConsumesPaging()
}

// ScrollsWithArrows opts the checklist into the frame's line-by-line arrow
// scroll; in full-screen mode the arrows fall through to handleLogKey and
// walk the log instead.
func (s *ExecStep) ScrollsWithArrows() bool {
	return s.hooks.Logs == nil || !s.log.Full()
}

// SuppressesSplit hands the log the whole frame while `f` has it
// full-screen; the checklist comes back the moment it is toggled off.
func (s *ExecStep) SuppressesSplit() bool {
	return s.log.Full() && s.hooks.Logs != nil
}

// PaneContent fills the split layout's right pane with the live log, in
// place of the context pane's step list.
func (s *ExecStep) PaneContent(width, height int) string {
	return s.log.RenderPane(width, height)
}

// paneCarriesLog reports whether the log has a pane of its own, in which
// case the checklist body carries no tail.
func (s *ExecStep) paneCarriesLog() bool {
	return s.hooks.Logs != nil && !s.log.Full() && s.SplitsFrame(flowStepCount)
}

// applyEvent updates node/row state for ev: a node change closes out the
// previous node first, a matched row advances to running or done, and an
// unmatched Reporter span degrades to the node's extra list.
func (s *ExecStep) applyEvent(ev *ExecEvent) {
	if len(s.nodes) == 0 {
		return
	}
	idx := s.nodeIndexFor(ev)
	if idx < 0 {
		idx = s.currentNode
	}
	if idx != s.currentNode {
		s.closeNode(&s.nodes[s.currentNode])
	}
	s.currentNode = idx
	np := &s.nodes[idx]
	if np.start.IsZero() {
		np.start = s.now()
	}

	row := matchRow(rowLabels(np.rows), ev)
	if row < 0 {
		if ev.Desc != "" && !ev.Done {
			np.extra = append(np.extra, ev.Desc)
		}
		return
	}
	switch {
	case ev.Done:
		np.rows[row].status = rowDone
		np.rows[row].took = ev.Took
		s.markEarlierRowsDone(np, row)
	default:
		if np.rows[row].status == rowPending {
			// A fresh row taking over "running" also takes over the extra
			// list: last render's leftover chatter belonged to the row
			// that just finished, not this one.
			np.extra = nil
			np.rows[row].status = rowRunning
			np.rows[row].start = s.now()
		}
		s.markEarlierRowsDone(np, row)
	}
}

// closeNode stamps np's end time and promotes any row still running or
// pending to done.
func (s *ExecStep) closeNode(np *nodeProgress) {
	now := s.now()
	if np.end.IsZero() {
		np.end = now
	}
	for i := range np.rows {
		promoteRowDone(&np.rows[i], now)
	}
}

// markEarlierRowsDone marks rows before active done: the backend runs rows
// strictly in order, so a later row starting implies the earlier ones finished.
func (s *ExecStep) markEarlierRowsDone(np *nodeProgress, active int) {
	now := s.now()
	for i := range active {
		promoteRowDone(&np.rows[i], now)
	}
}

// promoteRowDone closes a row that was still running or pending, backfilling
// a duration from its start when the caller never supplied one; a row that
// never started is marked implied so its duration renders "—".
func promoteRowDone(r *execRow, now time.Time) {
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

// finish closes out the node in flight when the run ends: a failure marks
// its running row failed, success closes it out like any earlier transition.
func (s *ExecStep) finish(err error) {
	if len(s.nodes) == 0 {
		return
	}
	np := &s.nodes[s.currentNode]
	if err != nil {
		now := s.now()
		for i := range np.rows {
			r := &np.rows[i]
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
	s.closeNode(np)
}

// nodeIndexFor resolves the node an event belongs to: an exact Node match
// first, then a whole-token description mention — bounded so worker1 never
// claims worker10's events on a 10+ node cluster.
func (s *ExecStep) nodeIndexFor(ev *ExecEvent) int {
	for i := range s.nodes {
		if ev.Node == s.nodes[i].name {
			return i
		}
	}
	if ev.Desc == "" {
		return -1
	}
	for i := range s.nodes {
		if mentionsNode(ev.Desc, s.nodes[i].name) {
			return i
		}
	}
	return -1
}

// mentionsNode reports whether desc contains name unextended — an occurrence
// followed by another digit is a longer sibling's name, not this node's.
func mentionsNode(desc, name string) bool {
	for at := 0; at <= len(desc)-len(name); {
		i := strings.Index(desc[at:], name)
		if i < 0 {
			return false
		}
		end := at + i + len(name)
		if end >= len(desc) || desc[end] < '0' || desc[end] > '9' {
			return true
		}
		at += i + 1
	}
	return false
}

func rowLabels(rows []execRow) []string {
	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].label
	}
	return out
}

// InterceptBack makes the execution screen forward-only: esc must never
// orphan the event pump mid-mutation (the runner goroutine would block on
// a full channel) or re-arm a listener on a finished run. Closing an open
// filter input is the one thing the key does here, and it has to happen on
// this hook: the frame consumes esc before the step's own Update could see it.
func (s *ExecStep) InterceptBack() bool {
	s.log.CancelFilter()
	return true
}

// InterceptQuit turns the first ctrl+c into a graceful cancel (the marker
// stays, the backend unwinds) and lets the second one force-quit.
func (s *ExecStep) InterceptQuit() bool {
	if s.cancelRequested || s.finished {
		return false
	}
	s.cancelRequested = true
	if s.hooks.CancelOp != nil {
		s.hooks.CancelOp()
	}
	return true
}

// View renders the per-node gate checklist: a finished node collapses to a
// single line with its total, the running node stays expanded with
// right-aligned durations and a live elapsed reading, and untouched nodes
// show a bare pending bullet.
func (s *ExecStep) View(width, _ int) string {
	col := max(width-4, 1)
	s.log.ViewCol = col
	if s.log.Full() {
		// The headline rides above the full-screen log: progress and elapsed
		// are what an operator would otherwise lose by leaving the checklist.
		// The sink path rides with it — the file keeps every byte the ring
		// evicts.
		head := []string{s.headline(col)}
		if s.hooks.LogPath != "" {
			head = append(head, s.Styles().Dim.Render(tui.Truncate("full log: "+s.hooks.LogPath, col)))
		}
		return strings.Join(head, "\n") + "\n" + s.log.RenderFull(col, max(s.BodyHeight()-len(head), 2))
	}

	lines := []string{s.headline(col)}
	if s.cancelRequested && !s.finished {
		lines = append(lines, s.Styles().Warn.Render(lipgloss.Wrap(
			"cancel requested — finishing the current terraform/oc call safely…", col, "")))
	}

	s.focusLine = -1
	for i := range s.nodes {
		lines = s.appendNode(lines, i, col)
	}

	// The cancel clause only applies while a first ctrl+c would still cancel
	// gracefully — after completion there is nothing to cancel, and after a
	// requested cancel the next ctrl+c force-quits. The sink clause names
	// the real resolved path, or nothing at all when no file sink is open —
	// never a guess.
	footnote := "marker okd-install/" + node.OpMarkerFileName
	if s.hooks.LogPath != "" {
		footnote += " · full log " + s.hooks.LogPath
	}
	if !s.finished && !s.cancelRequested {
		footnote += " · ctrl+c cancels after the current gate"
	}

	s.tailRendered = false
	if !s.paneCarriesLog() {
		// The tail's budget is whatever body rows the checklist and the
		// chrome around the tail (its blank row, the LOG header, and the
		// footnote block) leave over, floored at logview.NarrowTailRows —
		// slack becomes evidence instead of blank rows.
		budget := max(logview.NarrowTailRows, s.BodyHeight()-len(lines)-4)
		if tail := s.log.RenderTail(col, budget); len(tail) > 0 {
			lines = append(lines, "")
			lines = append(lines, tail...)
			s.tailRendered = true
		}
	}

	lines = append(lines, "", s.Styles().Dim.Render(lipgloss.Wrap(footnote, col, "")))

	content := strings.Join(lines, "\n")
	s.lastLine = strings.Count(content, "\n")
	return content
}

// headline renders the run's progress and its live elapsed time,
// right-aligned at col.
func (s *ExecStep) headline(col int) string {
	total := len(s.nodes)
	current := min(s.currentNode+1, total)
	left := fmt.Sprintf("%s  %d / %d", opProgressLabel(s.st.Op, s.execRole()), current, max(total, 1))
	right := "elapsed " + fmtDur(s.now().Sub(s.started))
	return justify(s.Styles().Bold.Render(left), s.Styles().Dim.Render(right), col)
}

// appendNode renders node i onto lines: collapsed with its total once
// passed (or once the run has finished cleanly), expanded with its rows
// while current, or a bare pending bullet otherwise.
func (s *ExecStep) appendNode(lines []string, i, col int) []string {
	np := &s.nodes[i]
	switch {
	case i < s.currentNode || (s.finished && s.st.Result == nil):
		return append(lines, justify(
			s.Styles().Done.Render(tui.IconSuccess+" "+np.name),
			s.Styles().Dim.Render(fmtDur(np.end.Sub(np.start))),
			col,
		))
	case i == s.currentNode || nodeTouched(np):
		lines = append(lines, s.Styles().Active.Render(tui.IconActive+" "+np.name))
		for j := range np.rows {
			r := &np.rows[j]
			if r.status == rowRunning {
				s.focusLine = len(lines)
			}
			lines = append(lines, "    "+s.renderRow(r, col-4))
			if r.status == rowRunning && len(np.extra) > 0 {
				lines = append(lines, "    "+s.Styles().Dim.MaxWidth(col-4).Render("… "+np.extra[len(np.extra)-1]))
			}
		}
		return lines
	default:
		return append(lines, s.Styles().Pend.Render(tui.IconPending+" "+np.name))
	}
}

// renderRow renders one gate row: done and failed rows show their duration
// right-aligned, the running row shows a live one, pending rows show neither.
func (s *ExecStep) renderRow(r *execRow, col int) string {
	switch r.status {
	case rowDone:
		return justify(s.Styles().Done.Render(tui.IconSuccess+" "+r.label), s.Styles().Dim.Render(rowDur(r)), col)
	case rowFailed:
		return justify(s.Styles().Fail.Render(tui.IconError+" "+r.label), s.Styles().Dim.Render(rowDur(r)), col)
	case rowRunning:
		return justify(wizard.Spinner(s.frame)+s.Styles().Bold.Render(r.label), s.Styles().Dim.Render(fmtDur(s.now().Sub(r.start))), col)
	default:
		return s.Styles().Pend.Render(tui.IconPending + " " + r.label)
	}
}

func nodeTouched(np *nodeProgress) bool {
	for i := range np.rows {
		if np.rows[i].status != rowPending {
			return true
		}
	}
	return len(np.extra) > 0
}

// FocusedSpan reports the running row's line, falling back to the last
// rendered line once the run has finished or no row is running. A narrow
// frame follows the tail instead: the log rides at the bottom of the body,
// and a checklist longer than the viewport would otherwise park the newest
// line below the fold.
func (s *ExecStep) FocusedSpan() (wizard.LineSpan, bool) {
	if len(s.nodes) == 0 {
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

func opProgressLabel(op node.Op, role nodetypes.NodeRole) string {
	switch op {
	case node.OpResize:
		switch role {
		case nodetypes.RoleMaster:
			return "resizing masters"
		case nodetypes.RoleWorker:
			return "resizing workers"
		default:
			return "resizing nodes"
		}
	case node.OpAdd:
		return "adding workers"
	case node.OpRemove:
		return "removing worker"
	default:
		return "running " + string(op)
	}
}

// justify right-aligns right within width, ANSI-safely truncating left
// (never padding it) so the combined line is exactly width columns wide; an
// oversized right on its own is clamped rather than left to overflow.
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

// fmtDur renders d truncated to whole seconds, the exec screen's duration format.
func fmtDur(d time.Duration) string {
	return d.Truncate(time.Second).String()
}

// rowDur renders a settled row's duration: "—" for one whose completion was
// inferred rather than measured, "<1s" for a real sub-second measurement.
func rowDur(r *execRow) string {
	if r.implied {
		return "—"
	}
	if r.took < time.Second {
		return "<1s"
	}
	return fmtDur(r.took)
}

// ShortHelp explains the log viewport's keys plus the constrained ones: no
// esc, guarded ctrl+c — the label matches the deploy stream screen's own
// cancel hint verbatim (StreamStep.ShortHelp), so the two full-screen exec
// surfaces read as one system rather than two different verbs for the same
// gesture. wizard.LogHelp orders the surface's own keys for both.
func (s *ExecStep) ShortHelp() []wizard.KeyBinding {
	cancel := wizard.KeyBinding{Key: wizard.HelpCtrlC, Help: "cancel (twice to force-quit)"}
	if s.hooks.Logs == nil {
		return []wizard.KeyBinding{cancel}
	}
	return wizard.LogHelp(&s.log, cancel)
}

// ConsumesTextInput reports the open filter input, so the frame hands the
// step every printable key — its own vim scroll keys and the "?" overlay
// toggle included — instead of acting on them itself.
func (s *ExecStep) ConsumesTextInput() bool {
	return s.log.Filtering()
}
