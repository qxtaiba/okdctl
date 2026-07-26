package wizard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/config"
)

type guardedStep struct {
	nopStep
	intercepts bool
	calls      int
}

func (s *guardedStep) InterceptQuit() bool { s.calls++; return s.intercepts }

func TestQuitGuardInterceptsCtrlC(t *testing.T) {
	g := &guardedStep{nopStep: *newNopStep(), intercepts: true}
	m := NewModel([]WizardStep{g}, config.DefaultConfig())
	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if updated.(*Model).quitting {
		t.Fatal("guarded step must prevent quit on first ctrl+c")
	}
	if g.calls != 1 {
		t.Fatalf("InterceptQuit calls = %d, want 1", g.calls)
	}

	g.intercepts = false
	updated, _ = updated.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !updated.(*Model).quitting {
		t.Fatal("once the guard declines, ctrl+c must quit")
	}
}

func TestUnguardedStepQuitsOnCtrlC(t *testing.T) {
	m := NewModel([]WizardStep{newNopStep()}, config.DefaultConfig())
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !updated.(*Model).quitting {
		t.Fatal("steps without QuitGuard must keep the immediate-quit behavior")
	}
}
