package lifecycle

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

type opChoice struct {
	op     node.Op
	resume bool
	title  string
	desc   string
}

// OpStep is the lifecycle flow's entry screen: pick resize/add/remove, or
// resume an interrupted op when a marker exists.
type OpStep struct {
	wizard.BaseStep
	st  *State
	nav *wizard.SingleSelect
	ops []opChoice
}

// NewOpStep constructs the operation-select step, pinning a resume option
// first when st.Marker names an interrupted op.
func NewOpStep(st *State) *OpStep {
	var ops []opChoice
	if st.Marker != nil {
		ops = append(ops, opChoice{
			op:     st.Marker.Op,
			resume: true,
			title:  fmt.Sprintf("resume interrupted %s", st.Marker.Op),
			desc:   "re-enters at the recorded step; completed nodes\nare skipped via a read-only plan probe",
		})
	}
	ops = append(ops,
		opChoice{op: node.OpResize, title: "resize nodes",
			desc: "change per-role cpu/memory, rolled out one node\nat a time behind etcd/ceph health gates"},
		opChoice{op: node.OpAdd, title: "add workers",
			desc: "build + upload a per-node iso, revive the ignition\nserver, join and wait ready"},
		opChoice{op: node.OpRemove, title: "remove worker",
			desc: "cordon, drain, destroy the vm, delete the node\n(highest-numbered worker only)"},
	)

	titles := make([]string, len(ops))
	for i, o := range ops {
		titles[i] = o.title
	}
	selector := components.NewCompactSelector(titles)
	selector.SetWrap(false)

	return &OpStep{
		BaseStep: wizard.NewBaseStep(StepIDOp, "operation", ""),
		st:       st,
		nav:      wizard.NewSingleSelect(StepIDOp, selector, "enter"),
		ops:      ops,
	}
}

// Init returns nil; the step has no async startup work.
func (s *OpStep) Init() tea.Cmd {
	return nil
}

// Update handles up/down navigation and enter to advance.
func (s *OpStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	return s, s.nav.Update(msg)
}

// IsCentered returns true so the entry screen renders centered.
func (s *OpStep) IsCentered() bool {
	return true
}

// View renders the operation menu with the resume banner when a marker is
// present.
func (s *OpStep) View(width, height int) string {
	s.SetSize(width, height)

	titleStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	subtitleStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate400).Italic(true)

	content := titleStyle.Render("cluster lifecycle") + "\n\n"
	content += subtitleStyle.Render(fmt.Sprintf("manage nodes on cluster %q", s.st.Cfg.Cluster.Name)) + "\n\n"

	if s.st.Marker != nil {
		warnStyle := lipgloss.NewStyle().Foreground(tui.ColorWarning)
		content += warnStyle.Render(fmt.Sprintf("⚠ interrupted %s of %s — step: %s, recorded %s ago",
			s.st.Marker.Op, s.st.Marker.Target, s.st.Marker.Step,
			humanAge(time.Since(s.st.Marker.Timestamp)))) + "\n\n"
	}

	for i, o := range s.ops {
		content += s.renderOption(&o, i == s.nav.SelectedIndex())
		if i < len(s.ops)-1 {
			content += "\n\n"
		}
	}
	return content
}

func (s *OpStep) renderOption(o *opChoice, selected bool) string {
	var bullet, title string
	if selected {
		bullet = lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true).Render("●")
		title = lipgloss.NewStyle().Foreground(tui.ColorText).Bold(true).Render(o.title)
	} else {
		bullet = lipgloss.NewStyle().Foreground(tui.ColorSlate600).Render("○")
		title = lipgloss.NewStyle().Foreground(tui.ColorSlate300).Render(o.title)
	}
	descStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	out := bullet + " " + title
	for line := range strings.SplitSeq(o.desc, "\n") {
		out += "\n  " + descStyle.Render(line)
	}
	return out
}

// Apply records the chosen operation. Choosing a non-resume op while a
// marker exists arms Ack so the backend's foreign-marker refusal becomes an
// explicit operator choice instead of a mid-flow error. Resume seeds the
// marker's target so the hidden target step is not missed.
func (s *OpStep) Apply(_ *config.Config) error {
	c := s.ops[s.nav.SelectedIndex()]
	s.st.Op = c.op
	s.st.Resume = c.resume
	s.st.Ack = s.st.Marker != nil && !c.resume
	s.st.Scope = node.ResizeScope{}
	s.st.Target = ""
	if c.resume {
		switch c.op {
		case node.OpResize:
			s.st.Scope = node.ResizeScope{Node: s.st.Marker.Target}
		case node.OpRemove:
			s.st.Target = s.st.Marker.Target
		}
	}
	return nil
}

// humanAge renders a duration at minute precision without the trailing
// zero units time.Duration.String produces ("2h", not "2h0m0s").
func humanAge(d time.Duration) string {
	if d < time.Minute {
		return "under a minute"
	}
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%dm", h, m)
	}
}

// SetFocused propagates focus to the selector.
func (s *OpStep) SetFocused(focused bool) {
	s.BaseStep.SetFocused(focused)
	s.nav.SetFocused(focused)
}

// ShortHelp returns the entry screen's help bar.
func (s *OpStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: "↑↓", Help: "select"},
		{Key: wizard.HelpEnter, Help: wizard.HelpConfirm},
		{Key: wizard.HelpCtrlC, Help: wizard.HelpQuit},
	}
}
