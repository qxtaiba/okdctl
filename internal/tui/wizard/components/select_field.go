package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qxtaiba/okdctl/internal/tui"
)

// SelectField is a dropdown-style form field that cycles through predefined
// options using left/right keys. It renders with the same bordered box style
// as InputField for visual consistency.
type SelectField struct {
	Label   string
	Help    string
	Options []string

	selected  int
	focused   bool
	width     int
	isDefault bool
}

// NewSelectField builds a SelectField with the given label and option list.
func NewSelectField(label string, options []string) *SelectField {
	return &SelectField{
		Label:   label,
		Options: options,
	}
}

// Value returns the currently selected option, or "" when none.
func (f *SelectField) Value() string {
	if f.selected >= 0 && f.selected < len(f.Options) {
		return f.Options[f.selected]
	}
	return ""
}

// SetValue selects the first option equal to value and marks the field as
// user-modified. Unknown values are silently ignored.
func (f *SelectField) SetValue(value string) {
	for i, opt := range f.Options {
		if opt == value {
			f.selected = i
			f.isDefault = false
			return
		}
	}
}

// SetDefault sets the starting selection and marks the field as unchanged.
func (f *SelectField) SetDefault(value string) {
	f.isDefault = true
	for i, opt := range f.Options {
		if opt == value {
			f.selected = i
			return
		}
	}
}

// IsDefault reports whether the field still holds its initial default.
func (f *SelectField) IsDefault() bool {
	return f.isDefault
}

// Focus gives the field focus so arrow keys cycle options.
func (f *SelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

// Blur removes focus from the field.
func (f *SelectField) Blur() {
	f.focused = false
}

// IsFocused reports whether the field currently owns focus.
func (f *SelectField) IsFocused() bool {
	return f.focused
}

// SetWidth records the rendering width used when drawing the bordered box.
func (f *SelectField) SetWidth(width int) {
	f.width = width
}

// Validate always returns nil because selection is constrained to Options.
func (f *SelectField) Validate() error {
	return nil // always valid — constrained to options
}

// Error always returns nil: SelectField has no validator that can fail.
func (f *SelectField) Error() error {
	return nil
}

// SetOptions replaces the option list, preserving the current selection by
// name when possible and resetting to index 0 otherwise.
func (f *SelectField) SetOptions(options []string) {
	current := f.Value()
	f.Options = options
	// Preserve selection by name
	f.selected = 0
	for i, opt := range options {
		if opt == current {
			f.selected = i
			return
		}
	}
}

// Update handles left/right and h/l key presses to cycle through Options.
func (f *SelectField) Update(msg tea.Msg) (FormField, tea.Cmd) {
	if !f.focused || len(f.Options) == 0 {
		return f, nil
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			f.selected--
			if f.selected < 0 {
				f.selected = len(f.Options) - 1
			}
			f.isDefault = false
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			f.selected++
			if f.selected >= len(f.Options) {
				f.selected = 0
			}
			f.isDefault = false
		}
	}

	return f, nil
}

// View renders the field with its label and a bordered box showing the
// current option, optionally flanked by cycle indicators when focused.
func (f *SelectField) View() string {
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate300)
	hintStyle := lipgloss.NewStyle().Foreground(tui.ColorSlate500)

	// Label line
	labelText := strings.ToLower(f.Label)
	labelLine := labelStyle.Render(labelText)
	if f.Help != "" {
		labelLine += " " + hintStyle.Render("("+strings.ToLower(f.Help)+")")
	}
	if f.isDefault && f.Value() != "" {
		defaultIndicator := lipgloss.NewStyle().
			Foreground(tui.ColorSlate500).
			Italic(true).
			Render(" (default)")
		labelLine += defaultIndicator
	}

	// Content width
	contentWidth := f.width - 4
	if contentWidth < 20 {
		contentWidth = 40
	}

	// Border style
	var boxStyle lipgloss.Style
	if f.focused {
		boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorPrimary).
			Padding(0, 1).
			Width(contentWidth)
	} else {
		boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorSlate600).
			Padding(0, 1).
			Width(contentWidth)
	}

	// Content inside box
	val := f.Value()
	var content string
	if f.focused && len(f.Options) > 1 {
		content = "◂ " + val + " ▸"
	} else {
		content = "> " + val
	}

	return labelLine + "\n" + boxStyle.Render(content)
}
