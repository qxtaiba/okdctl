package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// KeyHint is a single key/help pair a field contributes to the step's footer
// while it holds focus.
type KeyHint struct {
	Key  string
	Help string
}

// KeyHinter is implemented by fields whose interaction keys are shown in the
// step's footer ribbon rather than inline in the field's own label.
type KeyHinter interface {
	KeyHints() []KeyHint
}

// MultiSelectField renders a checklist of chips in the shared field box;
// Value/SetValue use a comma-separated format. Space toggles the current
// chip; left/right/h/l/j/k move the cursor — up/down are reserved by
// DataDrivenStep for navigation, same constraint as SelectField's left/right.
type MultiSelectField struct {
	Label   string
	Help    string
	Options []string

	selected []bool
	cursor   int
	focused  bool
	width    int
}

// NewMultiSelectField returns a multi-select field with options unchecked.
func NewMultiSelectField(label string, options []string) *MultiSelectField {
	return &MultiSelectField{
		Label:    label,
		Options:  options,
		selected: make([]bool, len(options)),
	}
}

// Value returns a comma-separated list of the selected options.
func (f *MultiSelectField) Value() string {
	var parts []string
	for i, opt := range f.Options {
		if i < len(f.selected) && f.selected[i] {
			parts = append(parts, opt)
		}
	}
	return strings.Join(parts, ",")
}

// SetValue marks each option present in the comma-separated value as selected.
func (f *MultiSelectField) SetValue(value string) {
	f.selected = make([]bool, len(f.Options))
	if value == "" {
		return
	}
	chosen := make(map[string]bool)
	for _, v := range strings.Split(value, ",") {
		chosen[strings.TrimSpace(v)] = true
	}
	for i, opt := range f.Options {
		f.selected[i] = chosen[opt]
	}
}

// Focus gives the field keyboard focus.
func (f *MultiSelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

// Blur removes keyboard focus from the field.
func (f *MultiSelectField) Blur() {
	f.focused = false
}

// SetWidth records the width available to the field's box and help row.
func (f *MultiSelectField) SetWidth(width int) {
	f.width = width
}

// Validate always returns nil — any non-empty selection is valid.
func (f *MultiSelectField) Validate() error {
	return nil
}

// KeyHints returns the field's footer hints, shown while it holds focus.
func (f *MultiSelectField) KeyHints() []KeyHint {
	return []KeyHint{
		{Key: "space", Help: "toggle"},
		{Key: "←/→", Help: "move"},
	}
}

// Update handles left/right/h/l/j/k cursor movement and space to toggle the
// chip under the cursor.
func (f *MultiSelectField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused || len(f.Options) == 0 {
		return f, nil
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("left", "h", "k"))):
			f.cursor--
			if f.cursor < 0 {
				f.cursor = len(f.Options) - 1
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("right", "l", "j"))):
			f.cursor++
			if f.cursor >= len(f.Options) {
				f.cursor = 0
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("space"))):
			if f.cursor >= 0 && f.cursor < len(f.selected) {
				f.selected[f.cursor] = !f.selected[f.cursor]
			}
		}
	}

	return f, nil
}

// View renders the label and a bordered box of chips, wrapped to the box's
// inner width, with the cursor preceding the current chip.
func (f *MultiSelectField) View() string {
	label := labelStyle.Render(f.Label)

	outer := f.width
	content := f.chipsContent(max(outer-4, 1))
	box := fieldBox(content, outer, f.focused, false)

	out := label + "\n" + box
	if f.focused && f.Help != "" {
		out += "\n" + helpStyle.Width(f.width).Render(f.Help)
	}
	return out
}

// chipsContent renders every option as a cursor+checkbox+name chip, wrapping
// chip-by-chip to a new row whenever the next chip would push the row past
// innerWidth; a chip is never split mid-glyph.
func (f *MultiSelectField) chipsContent(innerWidth int) string {
	checkedStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	uncheckedStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	cursorStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)

	var rows []string
	var row strings.Builder
	rowWidth := 0

	for i, opt := range f.Options {
		cursor := "  "
		if f.focused && i == f.cursor {
			cursor = cursorStyle.Render("> ")
		}

		checkbox := uncheckedStyle.Render("[ ]")
		if i < len(f.selected) && f.selected[i] {
			checkbox = checkedStyle.Render("[" + tui.IconSuccess + "]")
		}
		chip := cursor + checkbox + " " + opt
		chipWidth := lipgloss.Width(chip)

		if row.Len() > 0 && rowWidth+2+chipWidth > innerWidth {
			rows = append(rows, row.String())
			row.Reset()
			rowWidth = 0
		}
		if row.Len() > 0 {
			row.WriteString("  ")
			rowWidth += 2
		}
		row.WriteString(chip)
		rowWidth += chipWidth
	}
	if row.Len() > 0 {
		rows = append(rows, row.String())
	}
	return strings.Join(rows, "\n")
}
