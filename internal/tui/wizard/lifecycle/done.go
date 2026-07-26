package lifecycle

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/node"
	"github.com/qxtaiba/okdctl/internal/render"
	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// DoneStep is the terminal screen: the wizard-native rendering of the CLI
// completion box (outcome, per-node verbs, next steps), or the failure
// summary with the resume hint.
type DoneStep struct {
	wizard.BaseStep
	st *State
}

// NewDoneStep constructs the completion step.
func NewDoneStep(st *State) *DoneStep {
	return &DoneStep{
		BaseStep: wizard.NewBaseStep(StepIDDone, "done", ""),
		st:       st,
	}
}

// ShouldShow gates the step to consented (executed) runs.
func (s *DoneStep) ShouldShow(_ *config.Config) bool {
	return s.st.Proceed
}

// Init returns nil; the step only renders the recorded outcome.
func (s *DoneStep) Init() tea.Cmd {
	return nil
}

// Update completes the wizard on enter.
func (s *DoneStep) Update(msg tea.Msg) (wizard.WizardStep, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.Code == tea.KeyEnter {
		return s, func() tea.Msg { return wizard.StepCompleteMsg{StepID: StepIDDone} }
	}
	return s, nil
}

// View renders the success or failure summary.
func (s *DoneStep) View(width, height int) string {
	s.SetSize(width, height)
	if s.st.Result != nil {
		return s.failureView()
	}
	return s.successView(width)
}

func (s *DoneStep) successView(width int) string {
	st := wizard.NewSectionStyles(width)
	okStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)

	out := okStyle.Render("✓ "+completionHeadline(s.st.Op)) + "\n\n"
	out += st.KVPair("cluster", s.st.Cfg.Cluster.Name) + "\n"
	if s.st.Elapsed > 0 {
		out += st.KVPair("elapsed", s.st.Elapsed.Truncate(time.Second).String()) + "\n"
	}
	out += "\n"

	if s.st.Plan != nil {
		entries := make([]wizard.KVEntry, 0, len(s.st.Plan.Nodes))
		for i := range s.st.Plan.Nodes {
			n := &s.st.Plan.Nodes[i]
			entries = append(entries, wizard.KVEntry{
				Label: n.Name,
				Value: string(n.Role) + "  " + completionVerb(n.Action),
			})
		}
		out += wizard.RenderSection(&st, "nodes", entries)

		if steps := render.NodeOpNextSteps(s.st.Plan); len(steps) > 0 {
			nextEntries := make([]wizard.KVEntry, 0, len(steps))
			for _, line := range steps {
				nextEntries = append(nextEntries, wizard.KVEntry{Label: "", Value: line})
			}
			out += wizard.RenderSection(&st, "next steps", nextEntries)
		}
	}

	out += dimStyle.Render("enter to exit")
	return out
}

func (s *DoneStep) failureView() string {
	failStyle := lipgloss.NewStyle().Foreground(tui.ColorError).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(tui.ColorText)
	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)

	return failStyle.Render("✗ "+string(s.st.Op)+" failed") + "\n\n" +
		textStyle.Render(s.st.Result.Error()) + "\n\n" +
		dimStyle.Render("the op marker was left in place — re-run 'okdctl node manage' or the\nmatching flag verb to resume at the recorded step") + "\n\n" +
		dimStyle.Render("enter to exit")
}

func completionHeadline(op node.Op) string {
	switch op {
	case node.OpAdd:
		return "worker(s) added"
	case node.OpRemove:
		return "worker removed"
	case node.OpResize:
		return "resize complete"
	default:
		return "operation complete"
	}
}

func completionVerb(a terraform.PlanAction) string {
	switch a {
	case terraform.PlanActionCreate:
		return "added"
	case terraform.PlanActionDelete:
		return "removed"
	case terraform.PlanActionUpdate:
		return "resized (power-cycled to realize the change)"
	default:
		return string(a)
	}
}

// ShortHelp returns the completion help bar.
func (s *DoneStep) ShortHelp() []wizard.KeyBinding {
	return []wizard.KeyBinding{
		{Key: wizard.HelpEnter, Help: "exit"},
	}
}
