package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// MultiSelectField renders a checklist of options where any number may be
// toggled. Value returns selected items as a comma-separated string;
// SetValue parses the same format. Use space to toggle and j/k to move the
// cursor (bare up/down are consumed by DataDrivenStep for inter-field
// navigation, same constraint as SelectField's left/right keys).
type MultiSelectField struct {
	Label   string
	Help    string
	Options []string

	selected     []bool
	cursor       int
	focused      bool
	isDefault    bool
	defaultValue string
}

func NewMultiSelectField(label string, options []string) *MultiSelectField {
	return &MultiSelectField{
		Label:    label,
		Options:  options,
		selected: make([]bool, len(options)),
	}
}

func (f *MultiSelectField) Value() string {
	var parts []string
	for i, opt := range f.Options {
		if i < len(f.selected) && f.selected[i] {
			parts = append(parts, opt)
		}
	}
	return strings.Join(parts, ",")
}

func (f *MultiSelectField) SetValue(value string) {
	f.selected = make([]bool, len(f.Options))
	f.isDefault = false
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

func (f *MultiSelectField) SetDefault(value string) {
	f.SetValue(value)
	f.defaultValue = value
	f.isDefault = true
}

func (f *MultiSelectField) IsDefault() bool {
	return f.isDefault
}

func (f *MultiSelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *MultiSelectField) Blur() {
	f.focused = false
}

func (f *MultiSelectField) IsFocused() bool {
	return f.focused
}

func (f *MultiSelectField) SetWidth(_ int) {}

func (f *MultiSelectField) Validate() error {
	return nil
}

func (f *MultiSelectField) Error() error {
	return nil
}

func (f *MultiSelectField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused || len(f.Options) == 0 {
		return f, nil
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("k"))):
			f.cursor--
			if f.cursor < 0 {
				f.cursor = len(f.Options) - 1
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys("j"))):
			f.cursor++
			if f.cursor >= len(f.Options) {
				f.cursor = 0
			}
		case key.Matches(keyMsg, key.NewBinding(key.WithKeys(" "))):
			if f.cursor >= 0 && f.cursor < len(f.selected) {
				f.selected[f.cursor] = !f.selected[f.cursor]
				f.isDefault = false
			}
		}
	}

	return f, nil
}

func (f *MultiSelectField) View() string {
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate300)
	hintStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)
	cursorStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	checkedStyle := lipgloss.NewStyle().Foreground(tui.ColorSuccess)
	uncheckedStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)

	labelText := strings.ToLower(f.Label)
	labelLine := labelStyle.Render(labelText)
	if f.Help != "" {
		labelLine += " " + hintStyle.Render("("+strings.ToLower(f.Help)+")")
	}
	if f.focused {
		labelLine += " " + hintStyle.Render("(j/k navigate, space toggle)")
	}

	var lines []string
	lines = append(lines, labelLine)

	for i, opt := range f.Options {
		var checkbox string
		if i < len(f.selected) && f.selected[i] {
			checkbox = checkedStyle.Render("[x]")
		} else {
			checkbox = uncheckedStyle.Render("[ ]")
		}
		var cursor string
		if f.focused && i == f.cursor {
			cursor = cursorStyle.Render("> ")
		} else {
			cursor = "  "
		}
		lines = append(lines, cursor+checkbox+" "+opt)
	}

	return strings.Join(lines, "\n")
}
