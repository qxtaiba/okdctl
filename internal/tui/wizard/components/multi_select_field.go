package components

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// KeyHint is a key/help pair a focused field contributes to the footer.
type KeyHint struct {
	Key  string
	Help string
}

// KeyHinter is implemented by fields whose keys show in the footer ribbon.
type KeyHinter interface {
	KeyHints() []KeyHint
}

// MultiSelectField renders a checklist of chips; Value and SetValue use a
// comma-separated format, space toggles and left/right move, since up/down
// belong to the form.
type MultiSelectField struct {
	Label   string
	Help    string
	Options []string

	selected []bool
	cursor   int
	focused  bool
	width    int
	// baseLen counts the constructor's options; later entries were injected by
	// SetValue as "(current)" chips
	baseLen int
	// loaded echoes SetValue's exact argument until the first toggle, so an
	// untouched field round-trips byte-identical
	loaded string
}

// NewMultiSelectField returns a field with options unchecked.
func NewMultiSelectField(label string, options []string) *MultiSelectField {
	return &MultiSelectField{
		Label:    label,
		Options:  options,
		selected: make([]bool, len(options)),
		baseLen:  len(options),
	}
}

// Value returns the loaded value verbatim while untouched, then the selected
// options in option order.
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

// SetValue checks each listed option; an entry the list does not offer joins
// it as a checked "(current)" chip rather than being pruned.
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

// Focus gives the field focus.
func (f *MultiSelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

// Blur removes focus.
func (f *MultiSelectField) Blur() {
	f.focused = false
}

// SetWidth records the available width.
func (f *MultiSelectField) SetWidth(width int) {
	f.width = width
}

// Check always returns nil; the step checks the pool as a whole.
func (f *MultiSelectField) Check() error {
	return nil
}

// Validate always returns nil.
func (f *MultiSelectField) Validate() error {
	return f.Check()
}

// KeyHints returns the field's footer hints.
func (f *MultiSelectField) KeyHints() []KeyHint {
	return []KeyHint{
		{Key: "space", Help: "toggle"},
		{Key: "←/→", Help: "move"},
	}
}

// Update moves the cursor on left/right and h/l/j/k and toggles on space.
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
				f.loaded = ""
			}
		}
	}

	return f, nil
}

// View renders the label and a box of chips wrapped to its inner width.
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

// chipCoreWidth is the cursor, checkbox and spacing around a chip's name.
const chipCoreWidth = 2 + 3 + 1

// chipsContent wraps chips at chip boundaries, ellipsizing a name that alone
// exceeds innerWidth, since fieldBox would otherwise re-wrap mid-word.
func (f *MultiSelectField) chipsContent(innerWidth int) string {
	checkedStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess())
	uncheckedStyle := lipgloss.NewStyle().Foreground(tui.ColorTextFaint())
	cursorStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary()).Bold(true)

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

// ellipsize clips s to maxWidth columns with "…".
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
