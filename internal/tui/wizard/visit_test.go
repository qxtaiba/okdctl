package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

func TestOldVisitCannotCompleteReenteredStep(t *testing.T) {
	step := &fakeStep{id: StepIDBasics}
	m := NewModel([]WizardStep{step}, &config.Config{})
	m.beginVisit()
	delayed := m.ownCommand(func() tea.Msg { return StepCompleteMsg{StepID: StepIDBasics} })
	m.beginVisit()
	_, cmd := m.Update(delayed())
	if cmd != nil || m.Result().Outcome == OutcomeCompleted {
		t.Fatal("obsolete visit completed current step")
	}
	current := m.ownCommand(func() tea.Msg { return StepCompleteMsg{StepID: StepIDBasics} })
	m.Update(current())
	if m.Result().Outcome != OutcomeCompleted {
		t.Fatal("current visit did not complete")
	}
}

func TestVisitCancelsDepartedStep(t *testing.T) {
	first := NewDataDrivenStep(&StepDefinition{ID: StepIDBasics})
	second := NewDataDrivenStep(&StepDefinition{ID: StepIDFiles})
	m := NewModel([]WizardStep{first, second}, &config.Config{})
	m.beginVisit()
	previous := first.Context()
	m.focusStep(1)
	if previous.Err() == nil || second.Context().Err() != nil {
		t.Fatal("visit cancellation ownership incorrect")
	}
	m.stopVisit()
	if second.Context().Err() == nil {
		t.Fatal("flow exit left active visit alive")
	}
}

func TestShutdownWaitsForCancelledCommand(t *testing.T) {
	step := NewDataDrivenStep(&StepDefinition{ID: StepIDFiles})
	m := NewModel([]WizardStep{step}, &config.Config{})
	m.beginVisit()
	started := make(chan struct{})
	finished := make(chan struct{})
	command := m.ownCommand(func() tea.Msg { close(started); <-step.Context().Done(); close(finished); return nil })
	go command()
	<-started
	m.shutdown()
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned before worker cleanup")
	}
	called := false
	m.ownCommand(func() tea.Msg { called = true; return nil })()
	if called {
		t.Fatal("queued work started after shutdown")
	}
}
