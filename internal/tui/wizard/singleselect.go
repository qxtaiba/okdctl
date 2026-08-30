package wizard

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// SingleSelect owns the key loop for single-select wizard steps: confirm
// emits StepCompleteMsg, up/down/j/k delegate to the selector, and OnNav
// runs after every navigation key.
type SingleSelect struct {
	// OnNav, when non-nil, runs after every nav key with the new index and option count.
	OnNav func(index, total int) tea.Cmd

	selector *components.CompactSelector
	stepID   StepID
	confirm  key.Binding
	navigate key.Binding
}

// NewSingleSelect wraps selector in the shared single-select key loop;
// confirmKeys are the keys that complete the step.
func NewSingleSelect(stepID StepID, selector *components.CompactSelector, confirmKeys ...string) *SingleSelect {
	return &SingleSelect{
		selector: selector,
		stepID:   stepID,
		confirm:  key.NewBinding(key.WithKeys(confirmKeys...)),
		navigate: key.NewBinding(key.WithKeys("up", "k", "down", "j")),
	}
}

// Update handles confirm and navigation keys; other messages are ignored.
func (s *SingleSelect) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(keyMsg, s.confirm):
		stepID := s.stepID
		return func() tea.Msg { return StepCompleteMsg{StepID: stepID} }
	case key.Matches(keyMsg, s.navigate):
		var cmd tea.Cmd
		s.selector, cmd = s.selector.Update(msg)
		if s.OnNav != nil {
			return tea.Batch(cmd, s.OnNav(s.selector.SelectedIndex(), s.selector.Len()))
		}
		return cmd
	}
	return nil
}

// SelectedIndex returns the wrapped selector's current selection index.
func (s *SingleSelect) SelectedIndex() int {
	return s.selector.SelectedIndex()
}

// SetFocused toggles keyboard focus on the wrapped selector.
func (s *SingleSelect) SetFocused(focused bool) {
	s.selector.SetFocused(focused)
}

// View renders the wrapped selector.
func (s *SingleSelect) View() string {
	return s.selector.View()
}
