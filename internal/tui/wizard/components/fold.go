package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// Foldable is implemented by a Collapsible section's first field, letting
// MultiSectionForm read the fold's sticky user latch and tell it which
// chrome to draw this render without mutating that latch.
type Foldable interface {
	FormField
	// Expanded reports whether the fold's sticky user latch is open — set
	// only by the field's own enter handling, never by the form.
	Expanded() bool
	// SetDisplayExpanded tells the fold which chrome to draw this render:
	// open when the section is rendering in full for any reason (the
	// sticky latch, a focused field inside, or a validation error inside),
	// closed otherwise.
	SetDisplayExpanded(open bool)
}

// FoldToggleField is the one-line, always-present control a Collapsible
// section's View renders in place of its own section head — a summary row
// naming the fold and (via Summary) echoing non-sensitive facts about the
// fields it hides while collapsed, a bare heading row while the section
// renders in full — with enter (via EnterConsumer) flipping its sticky
// expand latch independently of the form's own per-render display state
// (SetDisplayExpanded), so a focus- or error-forced expansion never
// mutates the latch itself.
type FoldToggleField struct {
	Label string
	// Summary, when set, supplies the facts a collapsed row echoes beside
	// the label; called fresh each render so it reflects sibling fields'
	// live values. Never return a fact whose Value is credential material.
	Summary func() []tui.FactRow

	expanded        bool
	displayExpanded bool
	focused         bool
	width           int
}

// NewFoldToggleField returns a fold toggle labeled label, collapsed by
// default, echoing summary (if non-nil) while collapsed.
func NewFoldToggleField(label string, summary func() []tui.FactRow) *FoldToggleField {
	return &FoldToggleField{Label: label, Summary: summary}
}

// Value never carries a config binding — the fold is a rendering control.
func (f *FoldToggleField) Value() string { return "" }

// SetValue is a no-op — the fold carries no bindable value.
func (f *FoldToggleField) SetValue(string) {}

// FieldLabel returns the fold's label.
func (f *FoldToggleField) FieldLabel() string { return f.Label }

// FieldHelp always returns "" — the fold has no focused-field help text.
func (f *FoldToggleField) FieldHelp() string { return "" }

// Focus gives the fold keyboard focus so enter can toggle it.
func (f *FoldToggleField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

// Blur removes focus from the fold.
func (f *FoldToggleField) Blur() { f.focused = false }

// SetWidth records the width available to the fold's row.
func (f *FoldToggleField) SetWidth(width int) { f.width = width }

// Check always returns nil — the fold itself is never invalid.
func (f *FoldToggleField) Check() error { return nil }

// Validate always returns nil — the fold itself is never invalid.
func (f *FoldToggleField) Validate() error { return nil }

// ConsumesEnter always reports true: enter toggles the fold rather than
// submitting the step, whether or not the fold is currently expanded.
func (f *FoldToggleField) ConsumesEnter() bool { return true }

// Expanded reports whether the fold's sticky user latch is open.
func (f *FoldToggleField) Expanded() bool { return f.expanded }

// SetDisplayExpanded tells the fold which chrome to draw this render,
// independent of the sticky latch Expanded reports.
func (f *FoldToggleField) SetDisplayExpanded(open bool) { f.displayExpanded = open }

// KeyHints returns the fold's footer hint, naming the action enter performs next.
func (f *FoldToggleField) KeyHints() []KeyHint {
	if f.expanded {
		return []KeyHint{{Key: "enter", Help: "collapse"}}
	}
	return []KeyHint{{Key: "enter", Help: "expand"}}
}

// Update flips the sticky expand latch on enter while focused.
func (f *FoldToggleField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused {
		return f, nil
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))) {
		f.expanded = !f.expanded
	}
	return f, nil
}

// View renders a one-line triangle-plus-label row, appending Summary's
// facts (colon-joined) while the section is rendering collapsed.
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
