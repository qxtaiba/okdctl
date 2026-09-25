package components

import (
	"slices"
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
	// baseLen is how many options the constructor supplied; entries past it
	// were injected by SetValue for loaded values the list does not offer,
	// rendered as extra checked chips labeled (current).
	baseLen int
	// loaded is SetValue's exact argument, echoed by Value until the first
	// toggle edits the selection, so an untouched field round-trips its
	// config value byte-identical whatever order it listed entries in.
	loaded string
}

// NewMultiSelectField returns a multi-select field with options unchecked.
func NewMultiSelectField(label string, options []string) *MultiSelectField {
	return &MultiSelectField{
		Label:    label,
		Options:  options,
		selected: make([]bool, len(options)),
		baseLen:  len(options),
	}
}

// Value returns the loaded value verbatim while the selection is untouched,
// then a comma-separated list of the selected options in option order.
func (f *MultiSelectField) Value() string {
	if f.loaded != "" {
		return f.loaded
	}
	var parts []string
	for i, opt := range f.Options {
		if i < len(f.selected) && f.selected[i] {
			parts = append(parts, opt)
		}
	}
	return strings.Join(parts, ",")
}

// FieldLabel returns the field's label.
func (f *MultiSelectField) FieldLabel() string { return f.Label }

// FieldHelp returns the field's help text.
func (f *MultiSelectField) FieldHelp() string { return f.Help }

// SetValue marks each option present in the comma-separated value as
// selected. An entry the option list does not offer is never dropped: it
// joins the list as an extra checked chip labeled (current), so a valid
// loaded config round-trips instead of being silently pruned to the catalog.
func (f *MultiSelectField) SetValue(value string) {
	f.Options = f.Options[:f.baseLen]
	f.selected = make([]bool, len(f.Options))
	f.loaded = ""
	if f.cursor >= len(f.Options) {
		f.cursor = max(len(f.Options)-1, 0)
	}
	if value == "" {
		return
	}
	f.loaded = value
	chosen := make(map[string]bool)
	var unknown []string
	for _, v := range strings.Split(value, ",") {
		v = strings.TrimSpace(v)
		if !chosen[v] && !slices.Contains(f.Options, v) {
			unknown = append(unknown, v)
		}
		chosen[v] = true
	}
	for i, opt := range f.Options {
		f.selected[i] = chosen[opt]
	}
	for _, v := range unknown {
		f.Options = append(f.Options, v)
		f.selected = append(f.selected, true)
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

// Check always returns nil — any non-empty selection is valid.
func (f *MultiSelectField) Check() error {
	return nil
}

// Validate always returns nil — any non-empty selection is valid.
func (f *MultiSelectField) Validate() error {
	return f.Check()
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
				// The selection is edited: Value regenerates from it now.
				f.loaded = ""
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
	box := fieldBox(content, outer, f.focused, false, false)

	out := label + "\n" + box
	if f.focused && f.Help != "" {
		out += "\n" + helpStyle.Width(f.width).Render(f.Help)
	}
	return out
}

// chipCoreWidth is the width of every chip's cursor ("> " or "  ") plus
// checkbox ("[✓]" or "[ ]") plus the space before the option name.
const chipCoreWidth = 2 + 3 + 1

// chipsContent renders every option as a cursor+checkbox+name chip, wrapping
// at chip boundaries to a new row whenever the next chip would push the row
// past innerWidth; a chip that alone would exceed innerWidth has its name
// ellipsized to fit one row, since fieldBox's own lipgloss Width() re-wraps
// (mid-word) any content line it receives that is still too wide.
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

		name := opt
		if i >= f.baseLen {
			name += " (current)"
		}
		if nameBudget := innerWidth - chipCoreWidth; lipgloss.Width(name) > nameBudget {
			name = ellipsize(name, nameBudget)
		}
		chip := cursor + checkbox + " " + name
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

// ellipsize rune-safely clips s to fit within maxWidth visible columns,
// appending "…" when it clips.
func ellipsize(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := string(runes[:i]) + "…"
		if lipgloss.Width(candidate) <= maxWidth {
			return candidate
		}
	}
	return "…"
}
