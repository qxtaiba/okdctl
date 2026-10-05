package lifecycle

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/logview"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// execModel builds the lifecycle flow on the exec screen with a seeded log
// ring — deployexec's streamModel, for this flow.
func execModel(t *testing.T) *wizard.Model {
	t.Helper()
	tui.SetTerminalWidth(120)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st, hooks := threeMasterState(), goldenHooks()
	m := wizard.NewFlowModel(NewSteps(st, hooks), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 120, 40)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDExec})
	seedExecMidRun(m, st)
	return m
}

// TestExecFilterInputOwnsTheFrameKeys pins the text-entry contract where it
// actually matters — at the frame. While the filter input is open the
// wizard's own vim scroll keys and its help-overlay toggle must reach the
// step as typed text, and esc must close the input rather than being
// swallowed whole by the forward-only back guard.
func TestExecFilterInputOwnsTheFrameKeys(t *testing.T) {
	m := execModel(t)
	m.Update(tea.KeyPressMsg{Code: logview.KeyFilter, Text: "/"})

	s, ok := m.CurrentStep().(*ExecStep)
	if !ok {
		t.Fatalf("current step is %T, want the exec screen", m.CurrentStep())
	}
	if !s.ConsumesTextInput() {
		t.Fatal("an open filter input must assert ConsumesTextInput")
	}

	for _, r := range []rune{'j', 'G', '?', 'g'} {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 120, 40))
	if strings.Contains(frame, "key bindings") {
		t.Error("\"?\" typed into the filter must not open the help overlay")
	}
	if !strings.Contains(frame, "/jG?g") {
		t.Errorf("every typed key must land in the pattern:\n%s", frame)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if s.ConsumesTextInput() {
		t.Error("esc must close the filter input")
	}
	if strings.Contains(tuitest.StripANSI(tuitest.RenderAt(t, m, 120, 40)), "filter:") {
		t.Error("esc must leave no committed filter behind")
	}
}

// TestExecFilterSurvivesTheEnterCommit keeps the committed pattern selecting
// rows after the input closes, with the frame's keys back to normal.
func TestExecFilterSurvivesTheEnterCommit(t *testing.T) {
	m := execModel(t)
	m.Update(tea.KeyPressMsg{Code: logview.KeyFilter, Text: "/"})
	for _, r := range "step-2" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	s, ok := m.CurrentStep().(*ExecStep)
	if !ok {
		t.Fatalf("current step is %T, want the exec screen", m.CurrentStep())
	}
	if s.ConsumesTextInput() {
		t.Error("a committed filter must stop consuming text input")
	}
	frame := tuitest.StripANSI(tuitest.RenderAt(t, m, 120, 40))
	if !strings.Contains(frame, "filter: step-2 (4/24)") {
		t.Errorf("the committed chip must persist:\n%s", frame)
	}
	if strings.Contains(frame, "step=step-19") {
		t.Errorf("a committed filter must drop non-matching rows:\n%s", frame)
	}
}
