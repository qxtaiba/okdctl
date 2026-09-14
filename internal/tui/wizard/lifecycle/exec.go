package lifecycle

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/nodetypes"
	"github.com/qxtaiba/okdctl/internal/tui"
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
	st    *State
	hooks Hooks

	events           chan ExecEvent
	started          time.Time
	now              func() time.Time // overridden in tests for a deterministic elapsed reading
	startedGoroutine bool
	nodes            []nodeProgress
	currentNode      int
	cancelRequested  bool
	finished         bool
	loadingSpinner   spinner.Model
	// focusLine and lastLine are recorded during View: the running row's
	// line, and the last line of the rendered content.
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

// NewExecStep constructs the live execution step.
func NewExecStep(st *State, hooks Hooks) *ExecStep {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(tui.ColorPrimary)

	return &ExecStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(StepIDExec,
			"execute", "", ""),
		st:             st,
		hooks:          hooks,
		events:         make(chan ExecEvent, 32),
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

// DisplayTitle names the header for the operation in progress.
func (s *ExecStep) DisplayTitle() string {
	return opProgressLabel(s.st.Op, s.execRole())
}

// ShouldShow gates the step to consented plans.
func (s *ExecStep) ShouldShow(_ *config.Config) bool {
	return s.st.Proceed
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
		s.events <- ExecEvent{Final: true, Err: err}
	}()

	return tea.Batch(s.loadingSpinner.Tick, s.listen())
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

func (s *ExecStep) listen() tea.Cmd {
	return func() tea.Msg { return execEventMsg{ev: <-s.events} }
}

// Update consumes execution events and spinner ticks; every non-final
// event re-arms the listen command and nudges the viewport to follow the
// running row.
func (s *ExecStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case execEventMsg:
		if msg.ev.Final {
			s.finished = true
			s.st.Executed = true
			s.st.Result = msg.ev.Err
			s.st.Elapsed = s.now().Sub(s.started)
			s.finish(msg.ev.Err)
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDExec} }
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
// a duration from its start when the caller never supplied one.
func promoteRowDone(r *execRow, now time.Time) {
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

func (s *ExecStep) nodeIndexFor(ev *ExecEvent) int {
	for i := range s.nodes {
		if ev.Node == s.nodes[i].name {
			return i
		}
		if ev.Desc != "" && strings.Contains(ev.Desc, s.nodes[i].name) {
			return i
		}
	}
	return -1
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
// a full channel) or re-arm a listener on a finished run.
func (s *ExecStep) InterceptBack() bool {
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
func (s *ExecStep) View(width, height int) string {
	s.SetSize(width, height)
	col := max(width-4, 1)

	lines := []string{s.headline(col)}
	if s.cancelRequested && !s.finished {
		lines = append(lines, s.warnStyle.Render(lipgloss.Wrap(
			"cancel requested — finishing the current terraform/oc call safely…", col, "")))
	}

	s.focusLine = -1
	for i := range s.nodes {
		lines = s.appendNode(lines, i, col)
	}

	footnote := "marker okd-install/" + node.OpMarkerFileName + " · ctrl+c cancels after the current gate"
	lines = append(lines, "", s.dimStyle.Render(lipgloss.Wrap(footnote, col, "")))

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
	return justify(s.boldStyle.Render(left), s.dimStyle.Render(right), col)
}

// appendNode renders node i onto lines: collapsed with its total once
// passed (or once the run has finished cleanly), expanded with its rows
// while current, or a bare pending bullet otherwise.
func (s *ExecStep) appendNode(lines []string, i, col int) []string {
	np := &s.nodes[i]
	switch {
	case i < s.currentNode || (s.finished && s.st.Result == nil):
		return append(lines, justify(
			s.doneStyle.Render(tui.IconSuccess+" "+np.name),
			s.dimStyle.Render(fmtDur(np.end.Sub(np.start))),
			col,
		))
	case i == s.currentNode || nodeTouched(np):
		lines = append(lines, s.activeStyle.Render(tui.IconActive+" "+np.name))
		for j := range np.rows {
			r := &np.rows[j]
			if r.status == rowRunning {
				s.focusLine = len(lines)
			}
			lines = append(lines, "    "+s.renderRow(r, col-4))
			if r.status == rowRunning && len(np.extra) > 0 {
				lines = append(lines, "    "+s.dimStyle.MaxWidth(col-4).Render("… "+np.extra[len(np.extra)-1]))
			}
		}
		return lines
	default:
		return append(lines, s.pendStyle.Render(tui.IconPending+" "+np.name))
	}
}

// renderRow renders one gate row: done and failed rows show their duration
// right-aligned, the running row shows a live one, pending rows show neither.
func (s *ExecStep) renderRow(r *execRow, col int) string {
	switch r.status {
	case rowDone:
		return justify(s.doneStyle.Render(tui.IconSuccess+" "+r.label), s.dimStyle.Render(fmtDur(r.took)), col)
	case rowFailed:
		return justify(s.failStyle.Render(tui.IconError+" "+r.label), s.dimStyle.Render(fmtDur(r.took)), col)
	case rowRunning:
		return justify(s.loadingSpinner.View()+s.boldStyle.Render(r.label), s.dimStyle.Render(fmtDur(s.now().Sub(r.start))), col)
	default:
		return s.pendStyle.Render(tui.IconPending + " " + r.label)
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
// rendered line once the run has finished or no row is running.
func (s *ExecStep) FocusedSpan() (wizard.LineSpan, bool) {
	if len(s.nodes) == 0 {
		return wizard.LineSpan{}, false
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

// ShortHelp explains the constrained keys: no esc, guarded ctrl+c.
func (s *ExecStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: wizard.HelpCtrlC, Help: "request graceful cancel (twice to force-quit)"},
	}
}
