package steps

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/components"
)

// singleSelect owns the key loop shared by wizard steps whose entire
// interaction is one single-select list: a confirm key emits
// wizard.StepCompleteMsg, up/down/j/k delegate to the wrapped
// CompactSelector, and the optional onNav hook runs after every
// navigation key (regardless of selector focus, matching the historical
// step behavior).
type singleSelect struct {
	selector *components.CompactSelector
	stepID   wizard.StepID
	confirm  key.Binding
	navigate key.Binding
	onNav    func(index, total int) tea.Cmd
}

func newSingleSelect(stepID wizard.StepID, selector *components.CompactSelector, confirmKeys ...string) *singleSelect {
	return &singleSelect{
		selector: selector,
		stepID:   stepID,
		confirm:  key.NewBinding(key.WithKeys(confirmKeys...)),
		navigate: key.NewBinding(key.WithKeys("up", "k", "down", "j")),
	}
}

func (s *singleSelect) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(keyMsg, s.confirm):
		stepID := s.stepID
		return func() tea.Msg { return wizard.StepCompleteMsg{StepID: stepID} }
	case key.Matches(keyMsg, s.navigate):
		var cmd tea.Cmd
		s.selector, cmd = s.selector.Update(msg)
		if s.onNav != nil {
			return tea.Batch(cmd, s.onNav(s.selector.SelectedIndex(), s.selector.Len()))
		}
		return cmd
	}
	return nil
}

func (s *singleSelect) SelectedIndex() int {
	return s.selector.SelectedIndex()
}

func (s *singleSelect) SetFocused(focused bool) {
	s.selector.SetFocused(focused)
}

func (s *singleSelect) View() string {
	return s.selector.View()
}
