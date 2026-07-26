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
	took   time.Duration
}

type nodeProgress struct {
	name string
	rows []execRow
	// extra collects unmatched Reporter descriptions verbatim so a new
	// backend step degrades visibly instead of silently.
	extra []string
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
	startedGoroutine bool
	nodes            []nodeProgress
	currentNode      int
	cancelRequested  bool
	finished         bool
	loadingSpinner   spinner.Model
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
		loadingSpinner: sp,
	}
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
	s.started = time.Now()
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
	rows := GateRows(s.st.Op, s.execRole(), s.st.SkipDrain)
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
// event re-arms the listen command.
func (s *ExecStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	switch msg := msg.(type) {
	case execEventMsg:
		if msg.ev.Final {
			s.finished = true
			s.st.Executed = true
			s.st.Result = msg.ev.Err
			s.st.Elapsed = time.Since(s.started)
			s.finishRows(msg.ev.Err)
			return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDExec} }
		}
		s.applyEvent(&msg.ev)
		listen := s.listen()
		return s, listen

	case spinner.TickMsg:
		if !s.finished {
			var cmd tea.Cmd
			s.loadingSpinner, cmd = s.loadingSpinner.Update(msg)
			return s, cmd
		}
	}
	return s, nil
}

// applyEvent attributes an event to a node and flips the matched row.
func (s *ExecStep) applyEvent(ev *ExecEvent) {
	if len(s.nodes) == 0 {
		return
	}
	idx := s.nodeIndexFor(ev)
	if idx < 0 {
		idx = s.currentNode
	}
	s.currentNode = idx
	np := &s.nodes[idx]

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
			np.rows[row].status = rowRunning
		}
		s.markEarlierRowsDone(np, row)
	}
}

// markEarlierRowsDone commits every row before the active one: the backend
// runs rows strictly in order, so activity on row N proves rows < N ended.
func (s *ExecStep) markEarlierRowsDone(np *nodeProgress, active int) {
	for i := range active {
		if np.rows[i].status == rowRunning || np.rows[i].status == rowPending {
			np.rows[i].status = rowDone
		}
	}
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

// finishRows settles the checklist at the end of the run: success marks
// everything done, failure marks the in-flight row failed.
func (s *ExecStep) finishRows(err error) {
	for i := range s.nodes {
		for j := range s.nodes[i].rows {
			r := &s.nodes[i].rows[j]
			if r.status == rowRunning {
				if err != nil {
					r.status = rowFailed
				} else {
					r.status = rowDone
				}
			} else if err == nil && r.status == rowPending {
				r.status = rowDone
			}
		}
	}
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

// View renders the per-node gate checklist.
func (s *ExecStep) View(width, height int) string {
	s.SetSize(width, height)

	titleStyle := lipgloss.NewStyle().Foreground(tui.ColorText).Bold(true)
	doneStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	failStyle := lipgloss.NewStyle().Foreground(tui.ColorError)
	pendStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate600)
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)

	var b strings.Builder
	b.WriteString(titleStyle.Render(s.headline()))
	b.WriteString("\n\n")

	for i := range s.nodes {
		np := &s.nodes[i]
		bullet := pendStyle.Render("○")
		suffix := dimStyle.Render("  pending")
		if i == s.currentNode || nodeTouched(np) {
			bullet = lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true).Render("●")
			suffix = ""
		}
		b.WriteString(bullet + " " + titleStyle.Render(np.name) + suffix + "\n")
		if i == s.currentNode || nodeTouched(np) {
			for j := range np.rows {
				b.WriteString("    " + s.renderRow(&np.rows[j], &doneStyle, &failStyle, &pendStyle) + "\n")
			}
			for _, extra := range np.extra {
				b.WriteString("    " + dimStyle.Render("… "+extra) + "\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("op marker: okd-install/" + node.OpMarkerFileName + " — if this run is interrupted,"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("re-running the same operation resumes at the recorded step"))
	if s.cancelRequested && !s.finished {
		b.WriteString("\n\n")
		b.WriteString(warnStyle.Render("cancel requested — finishing the current terraform/oc call safely…"))
	}
	return b.String()
}

func (s *ExecStep) renderRow(r *execRow, doneStyle, failStyle, pendStyle *lipgloss.Style) string {
	took := ""
	if r.took > 0 {
		took = "  " + r.took.Truncate(time.Second).String()
	}
	switch r.status {
	case rowDone:
		return doneStyle.Render("✓ "+r.label) + took
	case rowFailed:
		return failStyle.Render("✗ " + r.label)
	case rowRunning:
		return s.loadingSpinner.View() + r.label
	default:
		return pendStyle.Render("○ " + r.label)
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

func (s *ExecStep) headline() string {
	total := len(s.nodes)
	current := min(s.currentNode+1, total)
	return fmt.Sprintf("%s — node %d of %d", opProgressLabel(s.st.Op), current, max(total, 1))
}

func opProgressLabel(op node.Op) string {
	switch op {
	case node.OpResize:
		return "resizing nodes"
	case node.OpAdd:
		return "adding workers"
	case node.OpRemove:
		return "removing worker"
	default:
		return "running " + string(op)
	}
}

// ShortHelp explains the constrained keys: no esc, guarded ctrl+c.
func (s *ExecStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: wizard.HelpCtrlC, Help: "request graceful cancel (twice to force-quit)"},
	}
}
