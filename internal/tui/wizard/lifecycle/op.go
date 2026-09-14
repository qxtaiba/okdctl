package lifecycle

import (
	"fmt"
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

// opCardWidth caps the wrap budget for the entry screen's description and
// banner text: without a cap the text would grow with the frame, widening
// the step's measured content until the wizard's IsCentered math had no
// gutter left to center against.
const opCardWidth = 58

// OpStep is the lifecycle flow's entry screen: pick resize/add/remove, or
// resume an interrupted op when a marker exists.
type OpStep struct {
	wizard.BaseStep
	st  *State
	nav *wizard.SingleSelect
	ops []opChoice
	now func() time.Time // overridden in tests for a deterministic marker age
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
			desc:   "re-enters at the recorded step; completed nodes are skipped via a read-only plan probe",
		})
	}
	ops = append(ops,
		opChoice{
			op: node.OpResize, title: "resize nodes",
			desc: "change per-role cpu/memory, rolled out one node at a time behind etcd/ceph health gates",
		},
		opChoice{
			op: node.OpAdd, title: "add workers",
			desc: "build + upload a per-node iso, revive the ignition server, join and wait ready",
		},
		opChoice{
			op: node.OpRemove, title: "remove worker",
			desc: "cordon, drain, destroy the vm, delete the node (highest-numbered worker only)",
		},
	)

	titles := make([]string, len(ops))
	for i, o := range ops {
		titles[i] = o.title
	}
	selector := components.NewCompactSelector(titles)
	selector.SetWrap(false)

	return &OpStep{
		BaseStep: wizard.NewBaseStepWithDisplayTitle(StepIDOp, "operation", "choose an operation", ""),
		st:       st,
		nav:      wizard.NewSingleSelect(StepIDOp, selector, "enter"),
		ops:      ops,
		now:      time.Now,
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
		banner := fmt.Sprintf("%s interrupted %s of %s — step: %s, recorded %s ago",
			tui.IconWarning, s.st.Marker.Op, s.st.Marker.Target, s.st.Marker.Step,
			humanAge(s.now().Sub(s.st.Marker.Timestamp)))
		content += warnStyle.Render(lipgloss.Wrap(banner, min(width, opCardWidth)-4, "")) + "\n\n"
	}

	for i, o := range s.ops {
		content += s.renderOption(&o, i == s.nav.SelectedIndex(), width)
		if i < len(s.ops)-1 {
			content += "\n\n"
		}
	}
	return content
}

func (s *OpStep) renderOption(o *opChoice, selected bool, width int) string {
	var bullet, title string
	if selected {
		bullet = lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true).Render(tui.IconActive)
		title = lipgloss.NewStyle().Foreground(tui.ColorText).Bold(true).Render(o.title)
	} else {
		bullet = lipgloss.NewStyle().Foreground(tui.ColorSlate600).Render(tui.IconPending)
		title = lipgloss.NewStyle().Foreground(tui.ColorSlate300).Render(o.title)
	}
	desc := lipgloss.Wrap(o.desc, min(width, opCardWidth)-8, "")
	descStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500).PaddingLeft(2)
	return bullet + " " + title + "\n" + descStyle.Render(desc)
}

// Apply records the chosen operation. Choosing a non-resume op over an
// existing marker arms Ack — the backend's foreign-marker refusal becomes
// an explicit choice — and resume seeds the marker's target into scope.
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

// humanAge renders duration at minute precision, unlike time.Duration.String ("2h", not "2h0m0s").
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
