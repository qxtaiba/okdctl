package deployexec

import (
	"fmt"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
	"github.com/qxtaiba/okdctl/internal/tui/tuitest"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// failedModel drives the deploy flow through a mid-ignition failure onto the
// incident report, the only path that produces a frozen checklist.
func failedModel(t *testing.T) (*wizard.Model, *State) {
	t.Helper()
	tui.SetTerminalWidth(120)
	t.Cleanup(func() { tui.SetTerminalWidth(0) })

	st := streamState()
	m := wizard.NewFlowModel(NewSteps(st, goldenHooks()), st.Cfg, Chrome())
	_ = tuitest.RenderAt(t, m, 120, 40)
	m.Update(wizard.JumpToStepMsg{StepID: StepIDStream})
	seedFailedRun(m, st)
	return m, st
}

func reportOf(t *testing.T, m *wizard.Model) string {
	t.Helper()
	s, ok := m.CurrentStep().(*DoneStep)
	if !ok {
		t.Fatalf("current step is %T, want the done screen", m.CurrentStep())
	}
	return tuitest.StripANSI(s.View(116, 1000))
}

// TestIncidentReportShowsTheChecklistFrozenAtFailure pins the handover: the
// run's shape is the first thing the report shows, and the operator does not
// lose it on the step transition.
func TestIncidentReportShowsTheChecklistFrozenAtFailure(t *testing.T) {
	m, st := failedModel(t)
	if len(st.frozen) == 0 {
		t.Fatal("the stream screen must hand its checklist over on the final event")
	}

	out := reportOf(t, m)
	card := strings.Index(out, "ERROR")
	if card < 0 {
		t.Fatalf("report carries no error card:\n%s", out)
	}
	checklist := out[:card]
	for _, want := range []string{
		tui.IconSuccess + " prep",
		tui.IconError + " ignition",
		tui.IconPending + " verify",
	} {
		if !strings.Contains(checklist, want) {
			t.Errorf("frozen checklist missing %q:\n%s", want, checklist)
		}
	}
}

// TestIncidentReportNamesTheFailurePoint pins the facts row: the report says
// which step in which phase died, read off the frozen checklist rather than
// re-parsed out of the error text.
func TestIncidentReportNamesTheFailurePoint(t *testing.T) {
	m, _ := failedModel(t)
	out := reportOf(t, m)

	for _, want := range []string{"run_id", "failed step", "generate ignition", "phase", "ignition", "elapsed", "cause"} {
		if !strings.Contains(out, want) {
			t.Errorf("incident facts missing %q:\n%s", want, out)
		}
	}
}

// TestIncidentReportKeepsDestructiveMovesAsHandoffLines is the campaign's
// destroy-handoff ruling, pinned: a teardown is a command the operator types,
// never a key the screen that just failed answers to.
func TestIncidentReportKeepsDestructiveMovesAsHandoffLines(t *testing.T) {
	m, _ := failedModel(t)
	out := reportOf(t, m)

	for _, want := range []string{handoffResume, handoffFresh, handoffDestroy} {
		if !strings.Contains(out, want) {
			t.Errorf("next moves missing the %q handoff line:\n%s", want, out)
		}
	}

	s, ok := m.CurrentStep().(*DoneStep)
	if !ok {
		t.Fatalf("current step is %T, want the done screen", m.CurrentStep())
	}
	for _, b := range s.ShortHelp() {
		if strings.Contains(b.Help, "destroy") || strings.Contains(b.Help, "tear down") {
			t.Errorf("a destructive move must never be a key: %q → %q", b.Key, b.Help)
		}
	}
}

// TestIncidentReportCopiesTheRunID pins the one in-TUI action: the escape goes
// out and the report says so without claiming the terminal accepted it.
func TestIncidentReportCopiesTheRunID(t *testing.T) {
	m, st := failedModel(t)
	s, ok := m.CurrentStep().(*DoneStep)
	if !ok {
		t.Fatalf("current step is %T, want the done screen", m.CurrentStep())
	}

	if _, cmd := s.Update(tea.KeyPressMsg{Code: keyCopy, Text: "c"}); cmd != nil {
		t.Error("off-TTY the clipboard escape must degrade to nothing")
	}

	t.Setenv("CLICOLOR_FORCE", "1")
	tui.SetColorProfileFor(io.Discard)
	t.Cleanup(func() {
		t.Setenv("CLICOLOR_FORCE", "")
		tui.SetColorProfileFor(io.Discard)
	})

	_, cmd := s.Update(tea.KeyPressMsg{Code: keyCopy, Text: "c"})
	if cmd == nil {
		t.Fatal("c must emit a clipboard command")
	}
	// bubbletea keeps the clipboard message unexported, so the assertion reads
	// its type name and the payload it carries.
	msg := cmd()
	if !strings.Contains(fmt.Sprintf("%T", msg), "setClipboardMsg") {
		t.Errorf("c produced %T, want a clipboard write", msg)
	}
	if got := fmt.Sprint(msg); got != st.RunID {
		t.Errorf("clipboard payload = %q, want the run id %q", got, st.RunID)
	}
	if out := reportOf(t, m); !strings.Contains(out, "OSC 52") {
		t.Errorf("the report must name the mechanism, not claim success:\n%s", out)
	}
	if st.RunID == "" {
		t.Fatal("fixture is wrong: the run must carry an id to copy")
	}
}

// TestIncidentReportEnterStillExits keeps the terminal key working while the
// screen has grown a vocabulary of its own.
func TestIncidentReportEnterStillExits(t *testing.T) {
	m, _ := failedModel(t)
	s, ok := m.CurrentStep().(*DoneStep)
	if !ok {
		t.Fatalf("current step is %T, want the done screen", m.CurrentStep())
	}

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must complete the wizard")
	}
	// With a filter input open the same key commits the pattern instead.
	s.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if _, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Error("enter must commit the filter, not quit, while the input is open")
	}
}
