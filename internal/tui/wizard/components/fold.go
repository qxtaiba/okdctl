package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// Foldable is implemented by a Collapsible section's first field, exposing
// the sticky user latch apart from the per-render display state.
type Foldable interface {
	FormField
	// Expanded reports the sticky user latch, set only by the field itself
	Expanded() bool
	// SetDisplayExpanded picks this render's chrome without touching the latch
	SetDisplayExpanded(open bool)
}

// FoldToggleField is the one-line control a Collapsible section renders in
// place of its head; enter flips its sticky latch while the form's display
// state (SetDisplayExpanded) tracks focus and errors separately.
type FoldToggleField struct {
	Label string
	// Summary supplies the collapsed row's facts each render; never return
	// credential material
	Summary func() []tui.FactRow

	expanded        bool
	displayExpanded bool
	focused         bool
	width           int
}

// NewFoldToggleField returns a collapsed fold toggle echoing summary.
func NewFoldToggleField(label string, summary func() []tui.FactRow) *FoldToggleField {
	return &FoldToggleField{Label: label, Summary: summary}
}

// Value is always empty; the fold carries no binding.
func (f *FoldToggleField) Value() string { return "" }

// SetValue is a no-op.
func (f *FoldToggleField) SetValue(string) {}

// Focus gives the fold focus so enter can toggle it.
func (f *FoldToggleField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

// Blur removes focus.
func (f *FoldToggleField) Blur() { f.focused = false }

// SetWidth records the available width.
func (f *FoldToggleField) SetWidth(width int) { f.width = width }

// Check always returns nil.
func (f *FoldToggleField) Check() error { return nil }

// Validate always returns nil.
func (f *FoldToggleField) Validate() error { return nil }

// ConsumesEnter always reports true: enter toggles the fold, never submits.
func (f *FoldToggleField) ConsumesEnter() bool { return true }

// Expanded reports whether the sticky latch is open.
func (f *FoldToggleField) Expanded() bool { return f.expanded }

// SetDisplayExpanded picks this render's chrome independently of the latch.
func (f *FoldToggleField) SetDisplayExpanded(open bool) { f.displayExpanded = open }

// KeyHints names the action enter performs next.
func (f *FoldToggleField) KeyHints() []KeyHint {
	if f.expanded {
		return []KeyHint{{Key: "enter", Help: "collapse"}}
	}
	return []KeyHint{{Key: "enter", Help: "expand"}}
}

// Update flips the sticky latch on enter while focused.
func (f *FoldToggleField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))) {
		f.expanded = !f.expanded
	}
	return f, nil
}

// View renders the triangle-plus-label row with Summary's facts while
// collapsed.
func (f *FoldToggleField) View() string {
	icon := lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Render(tui.IconCaretRight)
	label := labelStyle.Render(f.Label)
	if f.focused {
		label = lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true).Render(f.Label)
	}
	line := icon + " " + label

	if f.displayExpanded || f.Summary == nil {
		return line
	}
	facts := f.Summary()
	if len(facts) == 0 {
		return line
	}
	rendered := tui.RenderFacts(facts, &tui.FactLayout{Leader: tui.FactLeaderColon})
	return line + "  " + helpStyle.Render(strings.Join(rendered, "  ·  "))
}
